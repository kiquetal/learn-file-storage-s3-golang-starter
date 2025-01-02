package main

import (
	"fmt"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
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

	fmt.Printf("uploading video for video %v by user %v\n", videoID, userID)
}
