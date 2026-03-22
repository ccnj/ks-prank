package initialize

import (
	"crypto/tls"
	"time"

	"github.com/go-resty/resty/v2"

	"ks-prank/config"
	glb "ks-prank/internal/global"
)

func InitHttpClient() {
	glb.HttpClient = resty.New().
		SetBaseURL(config.ConfIns.ServerURL).
		SetTimeout(10 * time.Second).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}
