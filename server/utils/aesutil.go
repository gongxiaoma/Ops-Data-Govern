package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"go.uber.org/zap"
	"io"
)

// EncryptAES 加密密钥
// @param cipherKey 加密串
// @param plaintext 明文密码
// @return ciphertext 密文密码
// @return error 错误
func EncryptAES(cipherKey []byte, plaintext []byte) (ciphertext string, _err error) {
	// 创建AES加密块，NewCipher该函数限制了输入k的长度必须为16、24或32
	block, _err := aes.NewCipher(cipherKey)
	if _err != nil {
		global.GVA_LOG.Error("无法创建AES加密块: ", zap.Error(_err))
		return "", _err
	}

	// 获取秘钥块的长度
	blockSize := block.BlockSize()

	// 调用匿名函数PKCS#7实现填充(补全码)
	plaintextBytes := func(plaintext []byte, blockSize int) []byte {
		//判断缺少几位长度，最少1，最多blockSize
		padding := blockSize - len(plaintext)%blockSize
		//补足位数，把切片[]byte{byte(padding)}复制padding个
		padText := bytes.Repeat([]byte{byte(padding)}, padding)
		return append(plaintext, padText...)
	}(plaintext, blockSize)

	// 生成一个随机的初始化向量（IV）
	cipherText := make([]byte, aes.BlockSize+len(plaintextBytes))
	iv := cipherText[:aes.BlockSize]
	if _, _err := io.ReadFull(rand.Reader, iv); _err != nil {
		global.GVA_LOG.Error("无法生成初始化向量: ", zap.Error(_err))
		return "", _err
	}

	// 加密模式
	mode := cipher.NewCBCEncrypter(block, iv)
	// 加密
	mode.CryptBlocks(cipherText[aes.BlockSize:], plaintextBytes)

	// 返回的密文是IV和加密后的文本的十六进制表示
	return hex.EncodeToString(cipherText), nil
}

// DecryptAES 解密密钥
// @param cipherKey 加密串
// @param ciphertext 加密后密码
// @return plaintext 明文密码
// @return error 错误
func DecryptAES(cipherKey []byte, ciphertext string) (plaintext string, _err error) {
	// 创建AES解密器
	block, _err := aes.NewCipher(cipherKey)
	if _err != nil {
		global.GVA_LOG.Error("无法创建 AES 加密块: ", zap.Error(_err))
		return "", _err
	}

	// 获取秘钥块的长度
	blockSize := block.BlockSize()

	// 解码十六进制密文
	cipherText, _err := hex.DecodeString(ciphertext)
	if _err != nil {
		global.GVA_LOG.Error("无法解码十六进制密文: ", zap.Error(_err))
		return "", _err
	}

	// 检查密文长度是否至少为 AES 块大小（用于存储 IV）
	if len(cipherText) < blockSize {
		global.GVA_LOG.Error("无法创建 AES 加密块: ", zap.Error(_err))
		return "", _err
	}

	// 提取初始化向量 IV（密文的前 blockSize 字节）
	iv := cipherText[:blockSize]
	cipherText = cipherText[blockSize:]

	// 创建CBC解密模式
	mode := cipher.NewCBCDecrypter(block, iv)

	// 解密后的明文需要至少和密文一样长（实际可能更短，因为填充会被移除）
	plaintextBytes := make([]byte, len(cipherText))

	// 解密
	mode.CryptBlocks(plaintextBytes, cipherText)

	// 调用匿名函数移除PKCS#7填充(去除补全码)
	plainText := func(data []byte) []byte {
		length := len(data)
		unpadding := int(data[length-1])
		return data[:(length - unpadding)]
	}(plaintextBytes)

	// 返回解密后的明文
	return string(plainText), nil
}
