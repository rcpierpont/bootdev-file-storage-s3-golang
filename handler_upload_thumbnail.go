package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)
	fileData, fileHeaders, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to parse file data", err)
		return
	}

	mediaType, _, err := mime.ParseMediaType(fileHeaders.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable parse mime type from header", err)
		return
	}
	/*
		imageData, err := io.ReadAll(fileData)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "unable to read image file data", err)
			return
		}*/

	vidMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "unable to find video metadata in database with ID provided", err)
		return
	}

	if userID != vidMetadata.UserID {
		respondWithError(w, http.StatusUnauthorized, "not authorized", err)
		return
	}

	thumbnailID := make([]byte, 32)
	rand.Read(thumbnailID)
	assetName := fmt.Sprintf("%v.%s", base64.StdEncoding.EncodeToString(thumbnailID), getFileExtension(mediaType))
	assetPath := filepath.Join(cfg.assetsRoot, assetName)
	thumbnailURL := fmt.Sprintf("http://localhost:8091/%s", assetPath)
	assetFile, err := os.Create(assetPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "invalid asset path", err)
		return
	}
	defer assetFile.Close()

	n, err := io.Copy(assetFile, fileData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to write file contents to asset location", err)
		return
	}
	if n < 1 {
		respondWithError(w, http.StatusInternalServerError, "no file contents to write", err)
		return
	}

	vidMetadata.ThumbnailURL = &thumbnailURL
	err = cfg.db.UpdateVideo(vidMetadata)

	respondWithJSON(w, http.StatusOK, database.Video{})
}

func getFileExtension(s string) string {
	if strings.Contains(s, "image") {
		return strings.Replace(s, "image/", "", -1)
	}
	return ""
}
