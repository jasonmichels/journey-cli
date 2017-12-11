package journey

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/go-playground/validator.v9"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// CSS Struct for tracking public data of a css object
type CSS struct {
	URL string `json:"url"`
}

// JS Struct for tracking public data of a js object
type JS struct {
	URL    string `json:"url"`
	RootID string `json:"rootID"`
}

// Urls The urls of the assets are tracking
type Urls struct {
	CSS []CSS `json:"css"`
	JS  []JS  `json:"js"`
}

// Publish Publish the journey urls to the package and version
func (urls *Urls) Publish(journey *Journey, uploader *s3manager.Uploader, wg *sync.WaitGroup) (*s3manager.UploadOutput, error) {
	defer wg.Done()
	log.Printf("Starting to upload static asset urls to this bucket: %v", journey.Bucket)

	key := journey.Name + "/" + journey.Version + "/journey-urls.json"

	data, err := json.Marshal(urls)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the journey urls into json")
	}

	// Upload the static assest urls to S3
	return uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(journey.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/javascript"),
	})
}

// Journey Represents the journey.json configuration
type Journey struct {
	Name        string `json:"name" validate:"required"`
	Version     string `json:"version" validate:"required"`
	RootID      string `json:"rootID" validate:"required"`
	Build       string `json:"build" validate:"required"`
	Manifest    string `json:"manifest" validate:"required"`
	Bucket      string `json:"bucket" validate:"required"`
	JourneyPath string `validate:"required"`
	CDNDomain   string `validate:"required"`
}

// Validate Validate the journey config is correct
func (j *Journey) Validate(validate *validator.Validate) error {
	return validate.Struct(j)
}

// GetAssetPath the abs path to the asset
func (j *Journey) GetAssetPath(path string) string {
	return j.Build + path
}

// GetAssetKey Get the key to use in s3 bucket
func (j *Journey) GetAssetKey(path string) string {
	return j.Name + "/" + j.Version + "/" + path
}

// ValidateVersionNotUsed Validate that the version is not already in use, we dont want to publish over something
func (j *Journey) ValidateVersionNotUsed(sess *session.Session) (bool, error) {

	if j.Version == "latest" {
		return true, fmt.Errorf("Version %v is a reserved version. Please update and try again", j.Version)
	}

	svc := s3.New(sess)
	input := &s3.HeadObjectInput{
		Bucket: aws.String(j.Bucket),
		Key:    aws.String(j.Name + "/" + j.Version + "/journey.json"),
	}

	_, err := svc.HeadObject(input)
	if err != nil {
		// I know we are returning ok, but if no item is found we can assume the version does not exist
		return true, err
	}

	return false, fmt.Errorf("Version %v/%v already exists, publishing failed", j.Name, j.Version)
}

// Publish Publish the assets using the journey configuration
func (j *Journey) Publish(assets map[string]string) error {
	sess, err := session.NewSession(&aws.Config{Region: aws.String("us-east-1")})
	if err != nil {
		log.Println("Error creating AWS session ", err)
		return err
	}

	// check to make sure a directory in S3 does not exist with the Version
	if ok, err := j.ValidateVersionNotUsed(sess); !ok {
		return err
	}
	log.Printf("Version %v/%v is NOT being used already", j.Name, j.Version)

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	urls := j.BuildJourneyUrls(assets)

	log.Printf("Getting ready to upload %v files...", len(assets)+2)
	var wg sync.WaitGroup
	wg.Add(len(assets) + 3)

	for _, v := range assets {
		go uploadToS3(j.Bucket, j.GetAssetPath(v), j.GetAssetKey(v), uploader, &wg)
	}

	// make sure to put the journey.json, and asset-manifest.json file into {bucket}/{name}/{version}/
	go uploadToS3(j.Bucket, j.Manifest, j.GetAssetKey("asset-manifest.json"), uploader, &wg)
	go uploadToS3(j.Bucket, j.JourneyPath, j.GetAssetKey("journey.json"), uploader, &wg)
	go urls.Publish(j, uploader, &wg)
	wg.Wait()

	return nil
}

// BuildJourneyUrls Build the Journey Urls struct to have a list of css and js objects
func (j *Journey) BuildJourneyUrls(assets map[string]string) *Urls {
	var urls Urls
	var css []CSS
	var js []JS

	for _, v := range assets {
		// URL structure https://changeme.cloudfront.net/{j.Name}/{j.Version}/path
		url := j.CDNDomain + j.GetAssetKey(v)

		switch ext := filepath.Ext(v); ext {
		case ".css":
			css = append(css, CSS{URL: url})
		case ".js":
			js = append(js, JS{URL: url, RootID: j.RootID})
		default:
			log.Printf("Do not support adding %v files to journey-urls.json", ext)
		}
	}

	urls.CSS = css
	urls.JS = js

	return &urls
}

// getContentType Get the content type of a file path
func getContentType(path string) string {
	ext := filepath.Ext(path)
	mimeType := mime.TypeByExtension(ext)

	if len(mimeType) <= 0 {
		mimeType = "application/octet-stream"
	}

	return mimeType
}

// uploadToS3 Take a file path and key and upload to S3
func uploadToS3(bucket string, path string, key string, uploader *s3manager.Uploader, wg *sync.WaitGroup) (*s3manager.UploadOutput, error) {
	defer wg.Done()
	log.Printf("Starting to upload %v, at this path: %v, to this bucket: %v", key, path, bucket)

	if len(path) <= 0 {
		log.Printf("Key: %v, does not have a path and will not be uploaded", key)
		return nil, nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		log.Printf("Key: %v, had an issue getting absolute file path and was not uploaded", key)
		return nil, err
	}

	f, err := os.Open(abs)
	if err != nil {
		log.Printf("Key: %v, was unable to be opened and will not be uploaded", key)
		return nil, err
	}
	defer f.Close()

	// Upload the file to S3.
	return uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        f,
		ContentType: aws.String(getContentType(abs)),
	})
}
