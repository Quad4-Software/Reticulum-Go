// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_RSG_HPP
#define RNS_RSG_HPP

#include <cstddef>
#include <cstdint>
#include <string_view>
#include <vector>

#include "rns/detail/c_api.hpp"
#include "rns/error.hpp"
#include "rns/identity.hpp"
#include "rns/span.hpp"
#include "rns/util.hpp"

namespace rns {

inline Result<std::vector<std::uint8_t>> rsg_create(const Identity &identity,
						    span<const std::uint8_t> message, bool embed) {
	std::size_t needed = 0;
	Error probe = map_code(rns_rsg_create(identity.handle(), message.data(), message.size(),
					      embed ? 1 : 0, nullptr, 0, &needed));
	if (probe != Error::Ok && probe != Error::Truncated) {
		return Result<std::vector<std::uint8_t>>(probe);
	}
	if (needed == 0) {
		return Result<std::vector<std::uint8_t>>(Error::Internal);
	}
	std::vector<std::uint8_t> out(needed);
	std::size_t written = 0;
	Error err = map_code(rns_rsg_create(identity.handle(), message.data(), message.size(),
					    embed ? 1 : 0, out.data(), out.size(), &written));
	if (err != Error::Ok) {
		return Result<std::vector<std::uint8_t>>(err);
	}
	out.resize(written);
	return Result<std::vector<std::uint8_t>>(std::move(out));
}

inline Error rsg_validate(span<const std::uint8_t> rsg, span<const std::uint8_t> message,
			  span<const std::uint8_t> required_signer_hash) {
	return map_code(rns_rsg_validate(rsg.data(), rsg.size(), message.data(), message.size(),
					 required_signer_hash.data(), required_signer_hash.size()));
}

inline Result<std::vector<std::uint8_t>> rsg_sign_file(const Identity &identity,
						      std::string_view path) {
	char path_buf[4096];
	const char *path_z = nullptr;
	Error cerr = util::require_cstring(path, path_buf, sizeof(path_buf), &path_z);
	if (cerr != Error::Ok) {
		return Result<std::vector<std::uint8_t>>(cerr);
	}
	std::size_t needed = 0;
	Error probe = map_code(rns_rsg_sign_file(identity.handle(), path_z, nullptr, 0, &needed));
	if (probe != Error::Ok && probe != Error::Truncated) {
		return Result<std::vector<std::uint8_t>>(probe);
	}
	if (needed == 0) {
		return Result<std::vector<std::uint8_t>>(Error::Internal);
	}
	std::vector<std::uint8_t> out(needed);
	std::size_t written = 0;
	Error err =
	    map_code(rns_rsg_sign_file(identity.handle(), path_z, out.data(), out.size(), &written));
	if (err != Error::Ok) {
		return Result<std::vector<std::uint8_t>>(err);
	}
	out.resize(written);
	return Result<std::vector<std::uint8_t>>(std::move(out));
}

inline Error rsg_verify_file(span<const std::uint8_t> rsg, std::string_view path,
			     span<const std::uint8_t> required_signer_hash) {
	char path_buf[4096];
	const char *path_z = nullptr;
	Error cerr = util::require_cstring(path, path_buf, sizeof(path_buf), &path_z);
	if (cerr != Error::Ok) {
		return cerr;
	}
	return map_code(rns_rsg_verify_file(rsg.data(), rsg.size(), path_z,
					    required_signer_hash.data(),
					    required_signer_hash.size()));
}

inline Result<std::vector<std::uint8_t>>
rsm_verify(span<const std::uint8_t> rsm, span<const std::uint8_t> required_signer_hash) {
	std::size_t needed = 0;
	Error probe = map_code(rns_rsm_verify(rsm.data(), rsm.size(), required_signer_hash.data(),
					      required_signer_hash.size(), nullptr, 0, &needed));
	if (probe != Error::Ok && probe != Error::Truncated) {
		return Result<std::vector<std::uint8_t>>(probe);
	}
	if (needed == 0) {
		return Result<std::vector<std::uint8_t>>(std::vector<std::uint8_t>{});
	}
	std::vector<std::uint8_t> out(needed);
	std::size_t written = 0;
	Error err = map_code(rns_rsm_verify(rsm.data(), rsm.size(), required_signer_hash.data(),
					    required_signer_hash.size(), out.data(), out.size(),
					    &written));
	if (err != Error::Ok) {
		return Result<std::vector<std::uint8_t>>(err);
	}
	out.resize(written);
	return Result<std::vector<std::uint8_t>>(std::move(out));
}

} // namespace rns

#endif
