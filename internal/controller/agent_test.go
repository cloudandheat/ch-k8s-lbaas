package controller

import (
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"testing"

	jwt "github.com/dgrijalva/jwt-go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
)

type mockSimplifiedHTTPClient struct {
	mock.Mock
	sharedSecret []byte
}

func (m *mockSimplifiedHTTPClient) Post(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	if contentType != "application/jwt" {
		return &http.Response{
			StatusCode: 415,
		}, nil
	}

	buf := bytes.NewBuffer([]byte{})
	io.Copy(buf, body)
	claims := &model.ConfigClaim{}
	token, err := jwt.ParseWithClaims(string(buf.Bytes()), claims, func(*jwt.Token) (interface{}, error) {
		return m.sharedSecret, nil
	})

	if err != nil || !token.Valid {
		return &http.Response{
			StatusCode: 401,
		}, nil
	}

	claims, ok := token.Claims.(*model.ConfigClaim)
	if !ok {
		return &http.Response{
			StatusCode: 400,
		}, nil
	}

	a := m.Called(url, contentType, claims.Config)
	resp_untyped := a.Get(0)
	if resp_untyped == nil {
		return nil, a.Error(1)
	}
	return resp_untyped.(*http.Response), a.Error(1)
}

type acFixture struct {
	t *testing.T

	client *mockSimplifiedHTTPClient

	sharedSecret []byte
	agents       []string
}

func newACFixture(t *testing.T) *acFixture {
	f := &acFixture{}
	f.t = t
	secret := make([]byte, 8)
	_, err := rand.Read(secret)
	if err != nil {
		panic(err.Error())
	}
	f.sharedSecret = secret
	f.agents = []string{"http://127.1.0.1", "http://127.1.0.2/subpath"}
	f.client = &mockSimplifiedHTTPClient{
		sharedSecret: secret,
	}
	return f
}

func (f *acFixture) newAgentController() *HTTPAgentController {
	return &HTTPAgentController{
		SharedSecret: f.sharedSecret,
		AgentURLs:    f.agents,
		Client:       f.client,
	}
}

func (f *acFixture) run(body func(c *HTTPAgentController)) {
	c := f.newAgentController()
	body(c)
	f.client.AssertExpectations(f.t)
}

type dummyBody struct{}

func (d *dummyBody) Close() error {
	return nil
}

func (d *dummyBody) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

func TestPushJWTViaHTTP(t *testing.T) {
	f := newACFixture(t)
	m := &model.LoadBalancer{}

	f.client.On("Post", "http://127.1.0.1/v1/apply", "application/jwt", *m).Return(&http.Response{StatusCode: 200, Body: &dummyBody{}}, nil).Times(1)
	f.client.On("Post", "http://127.1.0.2/subpath/v1/apply", "application/jwt", *m).Return(&http.Response{StatusCode: 200, Body: &dummyBody{}}, nil).Times(1)

	f.run(func(c *HTTPAgentController) {
		err := c.PushConfig(m)
		assert.Nil(t, err)
	})
}
