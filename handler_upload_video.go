package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	const maxMemory = 1 << 30
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to parse form data", err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

	vidMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "unable to find video metadata in database with ID provided", err)
		return
	}

	if userID != vidMetadata.UserID {
		respondWithError(w, http.StatusUnauthorized, "not authorized", err)
		return
	}

	vidData, vidHeaders, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to parse video file and header from form data", err)
	}
	defer vidData.Close()

	mediaType, _, err := mime.ParseMediaType(vidHeaders.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable parse mime type from header", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "invalid format - video file required", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "unable to write video to temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, vidData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to copy file data to temp", err)
		return
	}

	processedPath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "unable to process temp video for faststart", err)
		return
	}

	processedFile, err := os.Open(processedPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to determine video aspect ratio", err)
		return
	}
	defer os.Remove(processedFile.Name())
	defer processedFile.Close()

	aspectRatio, err := getVideoAspectRatio(processedPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to determine video aspect ratio", err)
		return
	}

	vidID := make([]byte, 32)
	rand.Read(vidID)
	vidKey := fmt.Sprintf("%s/%v.%s", aspectRatio, base64.URLEncoding.EncodeToString(vidID), getFileExtension(mediaType))
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &vidKey,
		Body:        processedFile,
		ContentType: &mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to upload video to s3", err)
		return
	}

	vidURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, vidKey)
	vidMetadata.VideoURL = &vidURL
	err = cfg.db.UpdateVideo(vidMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to write updated video metadata to db", err)
	}

	respondWithJSON(w, http.StatusOK, database.Video{})
}

func processVideoForFastStart(filePath string) (string, error) {
	outPath := filePath + ".processing"
	fmt.Printf("inpath: %s\noutpath: %s\n", filePath, outPath)
	var stderr bytes.Buffer

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outPath)
	cmd.Stderr = &stderr

	err := cmd.Run()
	fmt.Println(stderr.String())
	if err != nil {
		fmt.Println("unable to execute ffmpeg command")
		return "", err
	}

	return outPath, nil
}
