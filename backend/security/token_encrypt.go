package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// encPrefix помечает зашифрованные значения в БД.
// Значения без этого префикса считаются plaintext (обратная совместимость).
const encPrefix = "enc:"

// EncryptToken шифрует строку алгоритмом AES-256-GCM.
// key должен быть ровно 32 байта. Возвращает "enc:<base64(nonce+ciphertext)>".
// Если key == nil — возвращает plaintext без шифрования (dev-режим).
func EncryptToken(key []byte, plaintext string) (string, error) {
	if len(key) == 0 {
		return plaintext, nil
	}
	if len(key) != 32 {
		return "", errors.New("token encryption key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize()) // 12 байт
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// DecryptToken расшифровывает значение, зашифрованное EncryptToken.
// Если значение не начинается с "enc:" — возвращает его без изменений
// (поддержка старых plaintext-записей в БД).
// Если key == nil — возвращает значение как есть.
func DecryptToken(key []byte, stored string) (string, error) {
	if len(key) == 0 || len(stored) < len(encPrefix) || stored[:len(encPrefix)] != encPrefix {
		return stored, nil
	}

	data, err := base64.RawURLEncoding.DecodeString(stored[len(encPrefix):])
	if err != nil {
		return "", errors.New("invalid encrypted token format")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("encrypted token too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("token decryption failed (wrong key or corrupted data)")
	}
	return string(plaintext), nil
}
