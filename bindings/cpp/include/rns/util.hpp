// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_UTIL_HPP
#define RNS_UTIL_HPP

#include <cstdint>
#include <cstring>
#include <string>
#include <string_view>

#include "rns/error.hpp"
#include "rns/types.hpp"

namespace rns {
namespace util {

inline Error to_cstring(std::string_view in, char *buf, std::size_t buf_len, const char **out) {
	if (in.empty()) {
		*out = nullptr;
		return Error::Ok;
	}
	if (in.size() + 1 > buf_len) {
		return Error::InvalidArg;
	}
	std::memcpy(buf, in.data(), in.size());
	buf[in.size()] = '\0';
	*out = buf;
	return Error::Ok;
}

inline Error require_cstring(std::string_view in, char *buf, std::size_t buf_len,
			     const char **out) {
	if (in.empty()) {
		return Error::InvalidArg;
	}
	return to_cstring(in, buf, buf_len, out);
}

inline std::string_view cstring_field(const char *buf, std::size_t cap) {
	if (buf == nullptr || cap == 0) {
		return {};
	}
	std::size_t n = 0;
	while (n < cap && buf[n] != '\0') {
		++n;
	}
	return std::string_view(buf, n);
}

inline Result<std::string> hash_to_hex(const std::uint8_t *hash, std::size_t len) {
	static const char *hex = "0123456789abcdef";
	std::string out;
	out.resize(len * 2);
	for (std::size_t i = 0; i < len; ++i) {
		out[i * 2] = hex[hash[i] >> 4];
		out[i * 2 + 1] = hex[hash[i] & 0xf];
	}
	return Result<std::string>(std::move(out));
}

inline Result<std::string> hash_to_hex(const Hash &hash) {
	return hash_to_hex(hash.data(), hash.size());
}

inline int hex_nibble(char c) {
	if (c >= '0' && c <= '9') {
		return c - '0';
	}
	if (c >= 'a' && c <= 'f') {
		return c - 'a' + 10;
	}
	if (c >= 'A' && c <= 'F') {
		return c - 'A' + 10;
	}
	return -1;
}

inline Result<Hash> hex_to_hash(std::string_view hex_str) {
	if (hex_str.size() != hash_len * 2) {
		return Result<Hash>(Error::InvalidArg);
	}
	Hash out{};
	for (std::size_t i = 0; i < hash_len; ++i) {
		int hi = hex_nibble(hex_str[i * 2]);
		int lo = hex_nibble(hex_str[i * 2 + 1]);
		if (hi < 0 || lo < 0) {
			return Result<Hash>(Error::InvalidArg);
		}
		out[i] = static_cast<std::uint8_t>((hi << 4) | lo);
	}
	return Result<Hash>(out);
}

} // namespace util
} // namespace rns

#endif
