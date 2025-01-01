package main

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

	// reset the file pointer
	_, err = file.Seek(0, 0)

	videoInfo, err := cfg.db.GetVideo(videoID)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video", err)
		return
	}

	mimeType, _, err := mime.ParseMediaType(headerContentType)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to parse media type", err)
	}

	if mimeType != "image/jpeg" && mimeType != "image/png" {
		fmt.Printf("Invalid media type: %s", mimeType)
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}

	var newThumbnail thumbnail
	newThumbnail.data = fileContent
	newThumbnail.mediaType = headerContentType

	//create a new file

	var extensionFile = strings.Split(headerContentType, "/")[1]
	create, err := os.Create(fmt.Sprintf("%s/%s.%s", filepath.Clean(cfg.assetsRoot), videoID, extensionFile))

	if err != nil {
		fmt.Println("Error creating file", err)
		return
	}

	written, err := io.Copy(create, file)
	if err != nil {

		fmt.Println("Error writing file", err)
	}

	fmt.Println("Written", written, "bytes")

	var nameVideo = fmt.Sprintf("%s.%s", videoID, extensionFile)
	var dataURL = fmt.Sprintf("http://localhost:%s/%s/%s", cfg.port, filepath.Clean(cfg.assetsRoot), nameVideo)

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
