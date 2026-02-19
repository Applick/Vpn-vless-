package vpnclient

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"

	"github.com/skip2/go-qrcode"
)

func ConfigToQRBase64(config string) (string, error) {
	pngBytes, err := qrcode.Encode(config, qrcode.Medium, 320)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pngBytes), nil
}

func DecodeBase64PNG(encoded string) (image.Image, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return img, nil
}
