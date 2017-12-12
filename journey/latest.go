package journey

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// Latest Deal with latest version of package
type Latest struct {
}

// SetLatest Set this version to the latest version
func (l *Latest) SetLatest(j *Journey, sess *session.Session) (*s3.CopyObjectOutput, error) {
	svc := s3.New(sess)
	input := &s3.CopyObjectInput{
		Bucket:     aws.String(j.Bucket),
		CopySource: aws.String(j.Bucket + "/" + j.GetJourneyURLPath()),
		Key:        aws.String(j.Name + "/latest/journey-urls.json"),
	}

	return svc.CopyObject(input)
}
