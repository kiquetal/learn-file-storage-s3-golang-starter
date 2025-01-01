package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
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

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)
	file, _, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from file", err)
		return
	}

	fileHeader := make([]byte, 512)
	_, err = file.Read(fileHeader)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read file header", err)
		return
	}

	// Reset the file pointer
	_, err = file.Seek(0, 0)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to seek file", err)
		return
	}
	headerContentType := http.DetectContentType(fileHeader)
	fmt.Println("Content Type: ", headerContentType)

	//read all the file content into a byte slice
	fileBytes := make([]byte, 512)
	var fileContent []byte
	for {
		n, err := file.Read(fileBytes)
		if err == io.EOF {
			break
		}
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Unable to read file content", err)
			return
		}
		fileContent = append(fileContent, fileBytes[:n]...)
	}

	videoInfo, err := cfg.db.GetVideo(videoID)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video", err)
		return
	}

	var newThumbnail thumbnail
	newThumbnail.data = fileContent
	newThumbnail.mediaType = headerContentType

	var base64String = base64.StdEncoding.EncodeToString(newThumbnail.data)

	var dataURL = "data:" + headerContentType + ";base64," + base64String

	videoInfo.ThumbnailURL = &dataURL
	err = cfg.db.UpdateVideo(videoInfo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}
	defer func(file multipart.File) {
		err := file.Close()
		if err != nil {
			fmt.Println("Error closing file", err)
		}
	}(file)

	respondWithJSON(w, http.StatusOK, videoInfo)
}
