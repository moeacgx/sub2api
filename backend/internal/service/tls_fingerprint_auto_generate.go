package service

import (
	"fmt"
	"math/rand/v2"

	"github.com/Wei-Shaw/sub2api/internal/model"
)

// generateUniqueNodeJSProfile 基于 Node.js 24.x 模板随机扰动生成唯一指纹。
// 每次调用产生不同的 cipher 顺序 / 扩展顺序 / 签名算法顺序，
// 但整体特征仍然是合法的 Node.js TLS ClientHello。
func generateUniqueNodeJSProfile(accountID int64) *model.TLSFingerprintProfile {
	desc := fmt.Sprintf("Auto-generated unique profile for account %d", accountID)
	return &model.TLSFingerprintProfile{
		Name:                fmt.Sprintf("auto-%d", accountID),
		Description:         &desc,
		EnableGREASE:        false, // 真实 Node.js/Claude Code 不开 GREASE
		CipherSuites:        shuffledCipherSuites(),
		Curves:              shuffledCurves(),
		PointFormats:        []uint16{0}, // uncompressed — 所有 Node.js 版本一致
		SignatureAlgorithms: shuffledSignatureAlgorithms(),
		ALPNProtocols:       []string{"http/1.1"}, // Node.js 特征，不加 h2
		SupportedVersions:   []uint16{0x0304, 0x0303}, // TLS1.3, TLS1.2
		KeyShareGroups:      []uint16{0x001d},          // X25519
		PSKModes:            []uint16{1},               // psk_dhe_ke
		Extensions:          shuffledExtensions(),
	}
}

// shuffledCurves 随机排列椭圆曲线。
// X25519 始终在第一位（性能最佳，几乎所有 Node.js 版本都首选），
// P256 和 P384 的相对顺序随机。
func shuffledCurves() []uint16 {
	tail := []uint16{0x0017, 0x0018} // P256, P384
	if rand.IntN(2) == 0 {
		tail[0], tail[1] = tail[1], tail[0]
	}
	return append([]uint16{0x001d}, tail...) // X25519 always first
}

// shuffledCipherSuites 随机生成 cipher suite 列表。
// TLS 1.3 套件始终在最前面（Anthropic 会校验），内部顺序随机。
// TLS 1.2 套件按功能分组后组间和组内都随机排列。
// 可选地包含或排除某些 legacy cipher（模拟不同 Node.js 编译配置）。
func shuffledCipherSuites() []uint16 {
	// TLS 1.3 — 始终包含，内部随机排序
	tls13 := []uint16{0x1301, 0x1302, 0x1303}
	rand.Shuffle(len(tls13), func(i, j int) { tls13[i], tls13[j] = tls13[j], tls13[i] })

	// TLS 1.2 分组 — ECDHE-GCM / ECDHE-ChaCha / ECDHE-CBC / RSA-GCM / RSA-CBC
	// 每组内部 ECDSA/RSA 顺序随机
	ecdheGCM128 := randomPair(0xc02b, 0xc02f)  // ECDSA, RSA
	ecdheGCM256 := randomPair(0xc02c, 0xc030)  // ECDSA, RSA
	ecdheChaCha := randomPair(0xcca9, 0xcca8)   // ECDSA, RSA
	ecdheCBC128 := randomPair(0xc009, 0xc013)   // ECDSA, RSA
	ecdheCBC256 := randomPair(0xc00a, 0xc014)   // ECDSA, RSA
	rsaGCM := randomPair(0x009c, 0x009d)        // 128, 256
	rsaCBC := randomPair(0x002f, 0x0035)         // 128, 256

	// 组合 ECDHE-GCM（128 和 256 的相对位置随机）
	var ecdheGCM []uint16
	if rand.IntN(2) == 0 {
		ecdheGCM = append(ecdheGCM128, ecdheGCM256...)
	} else {
		ecdheGCM = append(ecdheGCM256, ecdheGCM128...)
	}

	// 组合 ECDHE-CBC（128 和 256 的相对位置随机）
	var ecdheCBC []uint16
	if rand.IntN(2) == 0 {
		ecdheCBC = append(ecdheCBC128, ecdheCBC256...)
	} else {
		ecdheCBC = append(ecdheCBC256, ecdheCBC128...)
	}

	// ChaCha 组在 ECDHE-GCM 前面还是后面 — 模拟不同 Node.js 版本偏好
	var ecdheBlock []uint16
	if rand.IntN(3) == 0 { // 1/3 概率 ChaCha 优先
		ecdheBlock = append(ecdheChaCha, ecdheGCM...)
	} else {
		ecdheBlock = append(ecdheGCM, ecdheChaCha...)
	}
	ecdheBlock = append(ecdheBlock, ecdheCBC...)

	// 可选：是否包含 RSA-CBC legacy cipher（模拟精简编译的 Node.js）
	var rsaBlock []uint16
	rsaBlock = append(rsaBlock, rsaGCM...)
	if rand.IntN(4) != 0 { // 3/4 概率包含 RSA-CBC
		rsaBlock = append(rsaBlock, rsaCBC...)
	}

	result := make([]uint16, 0, len(tls13)+len(ecdheBlock)+len(rsaBlock))
	result = append(result, tls13...)
	result = append(result, ecdheBlock...)
	result = append(result, rsaBlock...)
	return result
}

// shuffledSignatureAlgorithms 随机排列签名算法。
// 按 hash 强度分组（SHA256/SHA384/SHA512），组内 ECDSA/RSA-PSS/RSA-PKCS1 顺序随机，
// 组间顺序随机。可选包含 rsa_pkcs1_sha1（模拟旧版 Node.js）。
func shuffledSignatureAlgorithms() []uint16 {
	sha256 := shuffleU16([]uint16{0x0403, 0x0804, 0x0401}) // ecdsa, rsa_pss, rsa_pkcs1
	sha384 := shuffleU16([]uint16{0x0503, 0x0805, 0x0501})
	sha512 := shuffleU16([]uint16{0x0806, 0x0601}) // 无 ecdsa_sha512

	groups := [][]uint16{sha256, sha384, sha512}
	rand.Shuffle(len(groups), func(i, j int) { groups[i], groups[j] = groups[j], groups[i] })

	var result []uint16
	for _, g := range groups {
		result = append(result, g...)
	}
	// 50% 概率追加 rsa_pkcs1_sha1（旧版 Node.js 保留）
	if rand.IntN(2) == 0 {
		result = append(result, 0x0201)
	}
	return result
}

// shuffledExtensions 随机排列 TLS 扩展顺序。
// server_name(0) 始终在第一个（标准要求），其余扩展随机排列。
// 可选包含/排除 ECH(65037) 和 SCT(18) — 模拟不同 Node.js 版本。
func shuffledExtensions() []uint16 {
	// 核心扩展（始终包含）
	core := []uint16{
		23,    // extended_master_secret
		65281, // renegotiation_info
		10,    // supported_groups
		11,    // ec_point_formats
		35,    // session_ticket
		16,    // alpn
		5,     // status_request
		13,    // signature_algorithms
		51,    // key_share
		45,    // psk_key_exchange_modes
		43,    // supported_versions
	}

	// 可选扩展
	if rand.IntN(3) != 0 { // 2/3 概率包含 ECH
		core = append(core, 65037)
	}
	if rand.IntN(2) == 0 { // 50% 概率包含 SCT
		core = append(core, 18)
	}

	rand.Shuffle(len(core), func(i, j int) { core[i], core[j] = core[j], core[i] })

	// server_name 始终第一个
	result := []uint16{0}
	result = append(result, core...)
	return result
}

// randomPair 随机决定两个值的顺序
func randomPair(a, b uint16) []uint16 {
	if rand.IntN(2) == 0 {
		return []uint16{a, b}
	}
	return []uint16{b, a}
}

// shuffleU16 返回 uint16 切片的随机排列副本
func shuffleU16(s []uint16) []uint16 {
	out := make([]uint16, len(s))
	copy(out, s)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
