package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/jasonmichels/journey-cli/journey"

	"gopkg.in/go-playground/validator.v9"
)

var j journey.Journey
var assets map[string]string

const publish = "publish"

func loadConfig(path string, v interface{}) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	content, err := ioutil.ReadFile(abs)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, v)
}

func main() {

	journeyPath := flag.String("journey", "journey.json", "Location of the journey.json file")
	cmd := flag.String("cmd", publish, "Command to invoke, eg: publish")
	bucket := flag.String("bucket", "", "AWS S3 bucket")
	cdnDomain := flag.String("cdn", "", "AWS Cloudfront domain")
	region := flag.String("region", "us-east-1", "AWS region where bucket located")
	flag.Parse()

	if err := loadConfig(*journeyPath, &j); err != nil {
		log.Panic(err)
	}
	log.Println("Successfully loaded journey.json configuration")

	j.Bucket = *bucket
	j.JourneyPath = *journeyPath
	j.CDNDomain = *cdnDomain

	if err := j.Validate(validator.New()); err != nil {
		log.Panic(err)
	}

	// lets create a new aws config
	awsConfig := aws.Config{Region: aws.String(*region)}

	switch *cmd {
	case publish:
		if err := loadConfig(j.Manifest, &assets); err != nil {
			log.Panic(err)
		}
		log.Println("Successfully loaded Asset Manifest configuration")

		if err := j.Publish(assets, &awsConfig); err != nil {
			log.Panic(err)
		}
		log.Println("Finished publishing all assets to S3")
	default:
		log.Fatalf("Do not recognize command: %v", *cmd)
	}

	log.Println("Continue with your Journey!")
}
