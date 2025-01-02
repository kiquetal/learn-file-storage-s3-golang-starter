package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	"mime"
	"net/http"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	videoId := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoId)
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

	//get video metadata

	videodb, err := cfg.db.GetVideo(videoID)
	if err != nil {

		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}

	if videodb.UserID != userID {

		respondWithError(w, http.StatusUnauthorized, "User not authorized", nil)
		return
	}

	//parse video from request

	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {

		respondWithError(w, http.StatusBadRequest, "Unable to parse form", err)
		return

	}
	file, header, err := r.FormFile("video")
	if err != nil {

		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	fmt.Printf("the video has the length of %v\n", file)

	defer file.Close()

	//check header content type
	headerContentType := header.Header.Get("Content-Type")

	mime.ParseMediaType(headerContentType)
	fmt.Println("Content Type: ", headerContentType)

	//generate 32bit using random

	var randomId []byte = make([]byte, 32)
	_, err = rand.Read(randomId)
	if err != nil {
		return
	}

	var stringBase64 = base64.StdEncoding.EncodeToString(randomId)
	fmt.Println("Random ID: ", stringBase64)

	fmt.Printf("uploading video for video %v by user %v\n", videoID, userID)
}
