package aws

import (
	"context"
	"fmt"

	"github.com/1Password/shell-plugins/sdk"
	"github.com/1Password/shell-plugins/sdk/schema/fieldname"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
)

type STSProvisioner struct {
	TOTPCode  string
	MFASerial string
}

func (p STSProvisioner) Provision(ctx context.Context, in sdk.ProvisionInput, out *sdk.ProvisionOutput) {
	if region, ok := in.ItemFields[FieldNameDefaultRegion]; ok {
		out.AddEnvVar("AWS_DEFAULT_REGION", region)
	}

	var cached sts.Credentials
	if ok := in.Cache.Get("sts", &cached); ok {
		out.AddEnvVar("AWS_ACCESS_KEY_ID", *cached.AccessKeyId)
		out.AddEnvVar("AWS_SECRET_ACCESS_KEY", *cached.SecretAccessKey)
		out.AddEnvVar("AWS_SESSION_TOKEN", *cached.SessionToken)
		return
	}

	sess, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(in.ItemFields[fieldname.AccessKeyID], in.ItemFields[fieldname.SecretAccessKey], ""),
	})
	if err != nil {
		out.AddError(fmt.Errorf("could not start aws STS session: %s", err))
		return
	}
	stsProvider := sts.New(sess)
	input := &sts.GetSessionTokenInput{
		DurationSeconds: aws.Int64(900), // minimum expiration time - 15 minutes
		SerialNumber:    aws.String(p.MFASerial),
		TokenCode:       aws.String(p.TOTPCode),
	}

	result, err := stsProvider.GetSessionToken(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			err = aerr
			if aerr.Code() == sts.ErrCodeRegionDisabledException {
				err = fmt.Errorf(sts.ErrCodeRegionDisabledException+": %s", aerr.Error())
			}
		}

		out.AddError(err)
		return
	}

	out.AddEnvVar("AWS_ACCESS_KEY_ID", *result.Credentials.AccessKeyId)
	out.AddEnvVar("AWS_SECRET_ACCESS_KEY", *result.Credentials.SecretAccessKey)
	out.AddEnvVar("AWS_SESSION_TOKEN", *result.Credentials.SessionToken)

	out.Cache.Put("sts", result.Credentials, *result.Credentials.Expiration)
}

func (p STSProvisioner) Deprovision(ctx context.Context, in sdk.DeprovisionInput, out *sdk.DeprovisionOutput) {
	// Nothing to do here: environment variables get wiped automatically when the process exits.
}

func (p STSProvisioner) Description() string {
	return "Provision environment variables with temporary STS credentials AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN"
}
