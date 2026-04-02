package initialize

import (
	"crypto/tls"
	"time"

	"github.com/go-resty/resty/v2"

	glb "ks-prank/internal/global"
)

func InitHttpClient(serverURL string) {
	glb.HttpClient = resty.New().
		SetBaseURL(serverURL).
		SetTimeout(10 * time.Second).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}
