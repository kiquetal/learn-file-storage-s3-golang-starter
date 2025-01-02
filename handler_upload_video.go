package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
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

	mediaType, _, err1 := mime.ParseMediaType(headerContentType)
	if err1 != nil {
		return
	}
	fmt.Println("Content Type: ", headerContentType)

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid content type", nil)
		return
	}

	//generate 32bit using random

	var randomId []byte = make([]byte, 32)
	_, err = rand.Read(randomId)
	if err != nil {
		return
	}

	var stringBase64 = base64.StdEncoding.EncodeToString(randomId)

	var tempFile, _ = os.CreateTemp("", "tubely-upload.mp4")

	defer os.Remove(tempFile.Name())

	defer tempFile.Close()

	written, errFile := io.Copy(tempFile, file)
	if errFile != nil {
		return
	}

	tempFile.Seek(0, io.SeekStart)
	var keyFile = fmt.Sprintf("videos/%s.%s", stringBase64, strings.Split(mediaType, "/")[1])

	fmt.Printf("The key file is %v\n", keyFile)
	var putObjectInput = s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &keyFile,
		Body:        tempFile,
		ContentType: &headerContentType,
	}
	_, err = cfg.s3Client.PutObject(context.Background(), &putObjectInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video", err)
		return
	}

	// update db
	var s3URL = fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, keyFile)
	videodb.VideoURL = &s3URL
	err = cfg.db.UpdateVideo(videodb)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	fmt.Println("Written: ", written)

	fmt.Println("Random ID: ", stringBase64)

	fmt.Printf("uploading video for video %v by user %v\n", videoID, userID)
}
