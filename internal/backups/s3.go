package backups

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"syscall"
	"time"
)

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

var s3HTTPClient = &http.Client{
	Timeout: 30 * time.Minute,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 15 * time.Second,
			// SSRF koruması: Control, DNS çözümü SONRASI, TCP bağlantısı kurulmadan
			// hemen önce çağrılır — bu yüzden "önce hostname'i kontrol et" deseninin
			// aksine DNS rebinding'e karşı da korur (kontrol ve bağlantı aynı IP
			// üzerinde, aralarında ayrı bir çözüm adımı yok).
			Control: func(_, address string, c syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				ip := net.ParseIP(host)
				if ip == nil {
					return fmt.Errorf("SSRF koruması: adres çözümlenemedi: %s", host)
				}
				if ipYasakli(ip) {
					return fmt.Errorf("SSRF koruması: yerel/özel ağ adresine bağlantı engellendi: %s", ip)
				}
				return nil
			},
		}).DialContext,
	},
}

func s3Endpoint(d *Destination) (*url.URL, error) {
	raw := strings.TrimSpace(d.Endpoint)
	if raw == "" {
		if d.Tip == "b2" {
			return nil, fmt.Errorf("backblaze S3 endpoint zorunlu")
		}
		region := d.Region
		if region == "" {
			region = "us-east-1"
		}
		if region == "us-east-1" {
			raw = "https://s3.amazonaws.com"
		} else {
			raw = "https://s3." + region + ".amazonaws.com"
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("endpoint geçerli bir HTTPS adresi olmalı")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("endpoint query veya fragment içeremez")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u, nil
}

func s3RequestURL(d *Destination, objectName string) (*url.URL, error) {
	u, err := s3Endpoint(d)
	if err != nil {
		return nil, err
	}
	bucket := strings.TrimSpace(d.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("bucket zorunlu")
	}
	parts := []string{strings.Trim(u.Path, "/")}
	if d.PathStyle {
		parts = append(parts, bucket)
	} else {
		u.Host = bucket + "." + u.Host
	}
	prefix := strings.Trim(d.UzakDizin, "/")
	if prefix != "" {
		parts = append(parts, prefix)
	}
	if objectName != "" {
		parts = append(parts, objectName)
	}
	u.Path = "/" + path.Join(parts...)
	return u, nil
}

func uploadS3Object(ctx context.Context, d *Destination, localPath, objectName string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		_ = f.Close()
		return fmt.Errorf("yedek hash: %w", err)
	}
	info, err := f.Stat()
	_ = f.Close()
	if err != nil {
		return err
	}
	payloadHash := hex.EncodeToString(hash.Sum(nil))
	u, err := s3RequestURL(d, objectName)
	if err != nil {
		return err
	}
	body, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer body.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), body)
	if err != nil {
		return err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", "application/gzip")
	signS3Request(req, d, payloadHash, time.Now().UTC())
	return doS3Request(req)
}

func testS3Connection(ctx context.Context, d *Destination) error {
	u, err := s3RequestURL(d, "")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	if err != nil {
		return err
	}
	signS3Request(req, d, emptySHA256, time.Now().UTC())
	return doS3Request(req)
}

func downloadS3Object(ctx context.Context, d *Destination, objectName, localPath string) error {
	u, err := s3RequestURL(d, objectName)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	signS3Request(req, d, emptySHA256, time.Now().UTC())
	resp, err := s3HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("S3 %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	tmp := localPath + ".part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, localPath)
}

func deleteS3Object(ctx context.Context, d *Destination, objectName string) error {
	u, err := s3RequestURL(d, objectName)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return err
	}
	signS3Request(req, d, emptySHA256, time.Now().UTC())
	return doS3Request(req)
}

func doS3Request(req *http.Request) error {
	resp, err := s3HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	detail := strings.TrimSpace(string(msg))
	if detail == "" {
		detail = resp.Status
	}
	return fmt.Errorf("S3 %s: %s", resp.Status, detail)
}

func signS3Request(req *http.Request, d *Destination, payloadHash string, now time.Time) {
	region := d.Region
	if region == "" {
		region = "us-east-1"
	}
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	canonicalHeaders := "host:" + req.URL.Host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := req.Method + "\n" + req.URL.EscapedPath() + "\n" +
		req.URL.Query().Encode() + "\n" + canonicalHeaders + "\n" +
		signedHeaders + "\n" + payloadHash
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" +
		hex.EncodeToString(requestHash[:])

	dateKey := hmacSHA256([]byte("AWS4"+d.Parola), date)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, "s3")
	signingKey := hmacSHA256(serviceKey, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+d.Kullanici+"/"+scope+
		", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
