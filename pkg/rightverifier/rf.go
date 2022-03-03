package rightverifier

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	uuid "github.com/gofrs/uuid"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/config"
)

const (
	httpClientTimeout = 3 * time.Second
)

type RightVerifierHandler struct {
	env config.Environment
}

func NewRightVerifierHandler(env config.Environment) *RightVerifierHandler {
	return &RightVerifierHandler{
		env: env,
	}
}

//return http code, 200 ok, 401 token outdated, 403 no access
func (hnd *RightVerifierHandler) Validate(id uuid.UUID, token string) (int, error) {

	var (
		u   *url.URL
		err error
	)

	rawUrl := fmt.Sprintf("%s%s", hnd.env.RightVerifURL, id.String())

	if u, err = url.Parse(rawUrl); err != nil {
		return 403, fmt.Errorf("rightVerifURL parse error: %w", err)
	}

	client := http.Client{
		Timeout: httpClientTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: hnd.env.RightVerifSkipTLS},
		},
	}
	header := make(http.Header)
	header.Add("Authorization", "Bearer "+token)
	req := &http.Request{
		Method: "GET",
		URL:    u,
		Header: header,
	}

	resp, err := client.Do(req)
	if err != nil {
		return 403, fmt.Errorf("cant connect to validate access right: %s : %w", id.String(), err)
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}
