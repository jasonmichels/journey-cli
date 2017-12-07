package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"gopkg.in/go-playground/validator.v9"
)

var journey Journey
var assets map[string]string
var validate *validator.Validate
var journeyPath *string

const publish = "publish"

func loadAssetManifestConfig(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	content, err := ioutil.ReadFile(abs)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, &assets)
}

func loadJourneyConfig(path string) error {

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	content, err := ioutil.ReadFile(abs)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, &journey)
}

// Journey Represents the journey.json configuration
type Journey struct {
	Name     string `json:"name" validate:"required"`
	Version  string `json:"version" validate:"required"`
	RootID   string `json:"rootID" validate:"required"`
	Build    string `json:"build" validate:"required"`
	Manifest string `json:"manifest" validate:"required"`
	Bucket   string `json:"bucket" validate:"required"`
}

// Validate Validate the journey config is correct
func (j *Journey) Validate() error {
	return validate.Struct(journey)
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
func (j *Journey) Publish() error {
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

	if err := loadAssetManifestConfig(j.Manifest); err != nil {
		return err
	}
	log.Println("Successfully loaded Asset Manifest configuration")

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	log.Printf("Getting ready to upload %v files...", len(assets)+2)
	var wg sync.WaitGroup
	wg.Add(len(assets) + 2)

	for _, v := range assets {
		go uploadToS3(j.Bucket, j.GetAssetPath(v), j.GetAssetKey(v), uploader, &wg)
	}

	// make sure to put the journey.json, and asset-manifest.json file into {bucket}/{name}/{version}/
	go uploadToS3(j.Bucket, j.Manifest, j.GetAssetKey("asset-manifest.json"), uploader, &wg)
	go uploadToS3(j.Bucket, *journeyPath, j.GetAssetKey("journey.json"), uploader, &wg)
	wg.Wait()

	return nil
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
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   f,
	})
}

func main() {

	journeyPath = flag.String("journey", "journey.json", "Location of the journey.json file")
	cmd := flag.String("cmd", publish, "Command to invoke, eg: publish")
	bucket := flag.String("bucket", "", "AWS S3 bucket")
	flag.Parse()

	if err := loadJourneyConfig(*journeyPath); err != nil {
		log.Panic(err)
	}
	log.Println("Successfully loaded journey.json configuration")

	journey.Bucket = *bucket

	validate = validator.New()
	if err := journey.Validate(); err != nil {
		log.Panic(err)
	}

	switch *cmd {
	case publish:
		if err := journey.Publish(); err != nil {
			log.Panic(err)
		}
		log.Println("Finished publishing all assets to S3")
	default:
		log.Fatalf("Do not recognize command: %v", *cmd)
	}

	log.Println("Continue with your Journey!")
}
