package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

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
	mediaType := fileHeaders.Header.Get("Content-Type")

	imageData, err := io.ReadAll(fileData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to read image file data", err)
		return
	}

	vidMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "unable to find video metadata in database with ID provided", err)
		return
	}

	if userID != vidMetadata.UserID {
		respondWithError(w, http.StatusUnauthorized, "not authorized", err)
		return
	}

	dataURL := fmt.Sprintf("data:%s;base64,%v", mediaType, base64.StdEncoding.EncodeToString(imageData))

	/*newThumbnail := thumbnail{
		data:      imageData,
		mediaType: mediaType,
	}
	videoThumbnails[videoID] = newThumbnail
	thumbnailURL := fmt.Sprintf("http://localhost:8091/api/thumbnails/%v", videoID)
	*/
	vidMetadata.ThumbnailURL = &dataURL
	err = cfg.db.UpdateVideo(vidMetadata)

	respondWithJSON(w, http.StatusOK, database.Video{})
}
