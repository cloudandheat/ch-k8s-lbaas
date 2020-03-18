package controller

import (
	"encoding/base64"
	goerrors "errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
)

type AgentController interface {
	PushConfig(m *model.LoadBalancer) error
}

type SimplifiedHTTPClient interface {
	Post(url, contentType string, body io.Reader) (resp *http.Response, err error)
}

type HTTPAgentController struct {
	AgentURLs     []string
	SharedSecret  []byte
	Client        SimplifiedHTTPClient
	TimeTolerance int
}

func NewHTTPAgentController(cfg config.Agents) (*HTTPAgentController, error) {
	agentURLs := make([]string, len(cfg.Agents))
	for i, agent := range cfg.Agents {
		if agent.URL == "" {
			return nil, fmt.Errorf("agent %d has unset url", i+1)
		}
		if !strings.HasPrefix(agent.URL, "http://") && !strings.HasPrefix(agent.URL, "https://") {
			return nil, fmt.Errorf("agents must have HTTP(S) url. offending agent %d: %s", i+1, agent.URL)
		}
		agentURLs[i] = agent.URL
	}

	if cfg.SharedSecret == "" {
		return nil, fmt.Errorf("shared-secret must not be empty")
	}

	sharedSecret, err := base64.StdEncoding.DecodeString(cfg.SharedSecret)
	if err != nil {
		return nil, fmt.Errorf("shared-secret must be valid base64: %s", err.Error())
	}

	if len(sharedSecret) < 12 {
		return nil, fmt.Errorf("shared-secret must have at least 12 bytes (got %d)", len(sharedSecret))
	}

	timeTolerance := cfg.TokenLifetime
	if timeTolerance == 0 {
		timeTolerance = 15
	}

	if timeTolerance < 0 || timeTolerance > 120 {
		return nil, fmt.Errorf("token-lifetime must be between 1 and 120 (got %d)", timeTolerance)
	}

	return &HTTPAgentController{
		AgentURLs:     agentURLs,
		SharedSecret:  sharedSecret,
		Client:        &http.Client{},
		TimeTolerance: timeTolerance,
	}, nil
}

func (c *HTTPAgentController) GenerateToken(m *model.LoadBalancer) (string, error) {
	claims := &model.ConfigClaim{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Duration(c.TimeTolerance) * time.Second).Unix(),
		},
		Config: *m,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(c.SharedSecret)
}

func (c *HTTPAgentController) PushConfig(m *model.LoadBalancer) error {
	errors := []error{}

	token, err := c.GenerateToken(m)
	if err != nil {
		return err
	}

	for _, agentUrl := range c.AgentURLs {
		fullUrl := fmt.Sprintf("%s/v1/apply", agentUrl)
		buf := strings.NewReader(token)
		resp, err := c.Client.Post(fullUrl, "application/jwt", buf)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if resp.StatusCode != 200 {
			errors = append(errors, fmt.Errorf(
				"failed to push config to agent %q: HTTP status %d",
				fullUrl,
				resp.StatusCode))
			continue
		}
	}

	switch len(errors) {
	case 0:
		return nil
	case 1:
		return errors[0]
	default:
		msg := &strings.Builder{}
		msg.WriteString("multiple errors while pushing config:\n")
		for _, err := range errors {
			msg.WriteString(err.Error())
			msg.WriteString("\n")
		}
		return goerrors.New(msg.String())
	}
}
