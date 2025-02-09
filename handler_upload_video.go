package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type AspectRatio string

const (
	AspectRatioLandscape AspectRatio = "landscape"
	AspectRatioPortrait  AspectRatio = "portrait"
	AspectRatioOther     AspectRatio = "other"
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

	var stringBase64 = base64.URLEncoding.EncodeToString(randomId)

	fmt.Printf("Url encoding is %v\n", stringBase64)

	var tempFile, _ = os.CreateTemp("", "tubely-upload.mp4")

	defer os.Remove(tempFile.Name())

	defer tempFile.Close()

	written, errFile := io.Copy(tempFile, file)
	if errFile != nil {
		return
	}

	tempFile.Seek(0, io.SeekStart)

	fmt.Printf("The path of the temp file is %v\n", tempFile.Name())
	fmt.Printf("The written file is %v\n", written)

	startFastFile, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		fmt.Printf("Error processing video for fast start %v\n", err)
		return
	}

	defer os.Remove(startFastFile)

	fileToUpload, _ := os.Open(startFastFile)
	//get aspect ratio
	fmt.Printf("Line 116\n")
	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get aspect ratio", err)
		return
	}

	var prefix string = fmt.Sprintf("%s/%s", aspectRatio, stringBase64)

	fmt.Printf("The prefix is %v\n", prefix)
	var keyFile = fmt.Sprintf("%s.%s", prefix, strings.Split(mediaType, "/")[1])

	fmt.Printf("The key file is %v\n", keyFile)
	var putObjectInput = s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &keyFile,
		Body:        fileToUpload,
		ContentType: &headerContentType,
	}
	_, err = cfg.s3Client.PutObject(context.Background(), &putObjectInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video", err)
		return
	}

	// update db
	//	var s3URL = fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, keyFile)
	var s3Tuple = fmt.Sprintf("%s,%s", cfg.s3Bucket, keyFile)
	//	videodb.VideoURL = &s3URL
	videodb.VideoURL = &s3Tuple
	fmt.Printf("The s3 tuple is %v before signed\n", s3Tuple)

	err = cfg.db.UpdateVideo(videodb)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	videoPresignUrl, e := cfg.dbVideoToSignedVideo(videodb)
	if e != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't sign video", e)
		return
	}

	respondWithJSON(w, http.StatusOK, videoPresignUrl)

}

func getVideoAspectRatio(filepath string) (AspectRatio, error) {

	fmt.Printf("The path of the file is %v\n", filepath)
	cmd := exec.Command("/usr/bin/ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filepath)
	output, err := cmd.Output()

	if err != nil {

		fmt.Println("Error running ffprobe", err)
		return "", err
	}
	var result map[string]interface{}

	err = json.Unmarshal(output, &result)
	if err != nil {
		fmt.Println("Error unmarshalling json", err)
		return "", err
	}

	streams := result["streams"].([]interface{})
	fmt.Printf("The streams are %v\n", streams)
	firstStream := streams[0].(map[string]interface{})

	width := firstStream["width"].(float64)
	height := firstStream["height"].(float64)

	gcd := func(a, b int) int {
		for b != 0 {
			a, b = b, a%b
		}
		return a
	}

	gcdValue := gcd(int(width), int(height))

	widthS := int(width) / int(gcdValue)
	heightS := int(height) / int(gcdValue)

	if widthS > heightS {
		return AspectRatioLandscape, nil
	} else {
		if widthS < heightS {
			return AspectRatioPortrait, nil
		} else {

			return AspectRatioOther, nil
		}
	}

}
func processVideoForFastStart(filepath string) (string, error) {
	outputFile := filepath + ".processing"
	fmt.Printf("The original file is %v\n", filepath)
	fmt.Printf("The output file is %v\n", outputFile)
	cmd := exec.Command("/usr/bin/ffmpeg", "-i", filepath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFile)
	err := cmd.Run()
	if err != nil {
		fmt.Println("Error running ffmpeg", err)
		return "", err
	}
	return outputFile, nil
}

func generatePResignedURL(s3client *s3.Client, bucket string, key string, expireTime time.Duration) (string, error) {

	s3PresingC := s3.NewPresignClient(s3client, func(p *s3.PresignOptions) {
		p.Expires = expireTime
	})

	req, e := s3PresingC.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})

	if e != nil {
		return "", e
	}
	return req.URL, nil

}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {

	if video.VideoURL == nil {
		return video, nil
	}

	s3Tuple := strings.Split(*video.VideoURL, ",")
	fmt.Printf("The s3 tuple is %v after split \n", s3Tuple)

	if len(s3Tuple) != 2 {
		return video, fmt.Errorf("Invalid S3 Tuple")
	}

	s3Bucket := s3Tuple[0]
	s3Key := s3Tuple[1]

	signedURL, err := generatePResignedURL(cfg.s3Client, s3Bucket, s3Key, 5*time.Minute)
	if err != nil {
		return video, err
	}

	video.VideoURL = &signedURL
	return video, nil
}
