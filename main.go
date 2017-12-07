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
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"gopkg.in/go-playground/validator.v9"
)

var journey Journey
var assetManifest AssetManifest
var validate *validator.Validate
var journeyPath *string

const publish = "publish"

type AssetManifest struct {
	MainJS     string `json:"main.js"`
	MainJSMap  string `json:"main.js.map"`
	MainCSS    string `json:"main.css"`
	MainCSSMap string `json:"main.css.map"`
}

func loadAssetManifestConfig(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	content, err := ioutil.ReadFile(abs)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, &assetManifest)
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

func (j *Journey) Publish() (bool, error) {
	sess, err := session.NewSession(&aws.Config{Region: aws.String("us-east-1")})
	if err != nil {
		fmt.Println("Error creating AWS session ", err)
		return false, err
	}

	// check to make sure a directory in S3 does not exist with the Version

	// if the directory does not exist, load the asset-manifest file into a struct
	// loop through each item in the asset manifest and upload the files
	// copy into {bucket}/{name}/{version}/
	if err := loadAssetManifestConfig(j.Manifest); err != nil {
		return false, err
	}

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	var wg sync.WaitGroup
	wg.Add(6)
	go uploadToS3(j.Bucket, j.GetAssetPath(assetManifest.MainCSS), j.GetAssetKey(assetManifest.MainCSS), uploader, &wg)
	go uploadToS3(j.Bucket, j.GetAssetPath(assetManifest.MainCSSMap), j.GetAssetKey(assetManifest.MainCSSMap), uploader, &wg)
	go uploadToS3(j.Bucket, j.GetAssetPath(assetManifest.MainJS), j.GetAssetKey(assetManifest.MainJS), uploader, &wg)
	go uploadToS3(j.Bucket, j.GetAssetPath(assetManifest.MainJSMap), j.GetAssetKey(assetManifest.MainJSMap), uploader, &wg)

	// make sure to put the journey.json, and asset-manifest.json file into {bucket}/{name}/{version}/
	go uploadToS3(j.Bucket, j.Manifest, j.GetAssetKey("asset-manifest.json"), uploader, &wg)
	go uploadToS3(j.Bucket, *journeyPath, j.GetAssetKey("journey.json"), uploader, &wg)
	wg.Wait()

	return true, nil
}

// uploadToS3 Take a file path and key and upload to S3
func uploadToS3(bucket string, path string, key string, uploader *s3manager.Uploader, wg *sync.WaitGroup) (*s3manager.UploadOutput, error) {
	defer wg.Done()

	if len(path) <= 0 {
		return nil, nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(abs)
	if err != nil {
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

	// add getting bucket from a flag or environment

	journeyPath = flag.String("journey", "journey.json", "Location of the journey.json file")
	cmd := flag.String("cmd", publish, "Command to invoke, eg: publish")
	bucket := flag.String("bucket", "", "AWS S3 bucket")
	flag.Parse()

	if err := loadJourneyConfig(*journeyPath); err != nil {
		panic(err)
	}

	journey.Bucket = *bucket

	validate = validator.New()
	if err := journey.Validate(); err != nil {
		panic(err)
	}

	//Call the command, right now just publish
	switch *cmd {
	case publish:
		journey.Publish()
	default:
		log.Println("Do not recognize command")
	}
}
