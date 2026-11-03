package backend

import (
	"context"
	"errors"
	"fmt"
	"os"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"golang.org/x/oauth2/google"
)

type Gcp struct {
	Ref     `yaml:",inline"`
	Project string `yaml:"namespace"`
}

func (g *Gcp) project(ctx context.Context) (string, error) {
	if g.Project != "" {
		return g.Project, nil
	}
	if p := os.Getenv("GOOGLE_CLOUD_PROJECT"); p != "" {
		return p, nil
	}
	creds, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		return "", err
	}
	if creds.ProjectID == "" {
		return "", errors.New("no project set and none found in default credentials")
	}
	return creds.ProjectID, nil
}

func (g *Gcp) Load(ctx context.Context) (string, error) {
	project, err := g.project(ctx)
	if err != nil {
		return "", err
	}

	c, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()

	res, err := c.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, g.Key, g.Version),
	})
	if err != nil {
		return "", err
	}
	return string(res.Payload.Data), nil
}

func init() {
	Register("gcp", func() Provider { return &Gcp{Ref: Ref{Version: "latest"}} })
}
