// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_IDENTITY_HPP
#define RNS_IDENTITY_HPP

#include <cstdint>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

#include "rns/detail/c_api.hpp"
#include "rns/error.hpp"
#include "rns/span.hpp"
#include "rns/types.hpp"
#include "rns/util.hpp"

namespace rns {

class Identity {
      public:
	Identity() noexcept : handle_(0) {}

	Identity(Identity &&other) noexcept : handle_(other.handle_) { other.handle_ = 0; }

	Identity &operator=(Identity &&other) noexcept {
		if (this != &other) {
			reset();
			handle_ = other.handle_;
			other.handle_ = 0;
		}
		return *this;
	}

	Identity(const Identity &) = delete;
	Identity &operator=(const Identity &) = delete;

	~Identity() { reset(); }

	static Result<Identity> generate() {
		std::uint64_t h = rns_identity_generate();
		if (h == 0) {
			return Result<Identity>(Error::Internal);
		}
		return Result<Identity>(Identity(h));
	}

	static Result<Identity> load(std::string_view path) {
		char path_buf[4096];
		const char *path_z = nullptr;
		Error cerr = util::require_cstring(path, path_buf, sizeof(path_buf), &path_z);
		if (cerr != Error::Ok) {
			return Result<Identity>(cerr);
		}
		std::uint64_t h = rns_identity_load(path_z);
		if (h == 0) {
			return Result<Identity>(Error::Io);
		}
		return Result<Identity>(Identity(h));
	}

	Error save(std::string_view path) const {
		char path_buf[4096];
		const char *path_z = nullptr;
		Error cerr = util::require_cstring(path, path_buf, sizeof(path_buf), &path_z);
		if (cerr != Error::Ok) {
			return cerr;
		}
		return map_code(rns_identity_save(handle_, path_z));
	}

	Result<std::string> hash() const {
		char buf[64];
		std::size_t written = 0;
		Error err = map_code(rns_identity_hash(handle_, buf, sizeof(buf), &written));
		if (err != Error::Ok) {
			return Result<std::string>(err);
		}
		if (written > sizeof(buf)) {
			written = sizeof(buf);
		}
		return Result<std::string>(std::string(buf, written));
	}

	Result<Hash> hash_bytes() const {
		Hash out{};
		std::size_t written = 0;
		Error err = map_code(rns_identity_hash_bytes(handle_, out.data(), out.size(), &written));
		if (err != Error::Ok) {
			return Result<Hash>(err);
		}
		if (written != hash_len) {
			return Result<Hash>(Error::Truncated);
		}
		return Result<Hash>(out);
	}

	Result<std::vector<std::uint8_t>> public_key() const {
		std::uint8_t buf[64];
		std::size_t written = 0;
		Error err = map_code(rns_identity_public_key(handle_, buf, sizeof(buf), &written));
		if (err != Error::Ok) {
			return Result<std::vector<std::uint8_t>>(err);
		}
		if (written > sizeof(buf)) {
			written = sizeof(buf);
		}
		return Result<std::vector<std::uint8_t>>(
		    std::vector<std::uint8_t>(buf, buf + written));
	}

	static Result<Identity> from_public_key(span<const std::uint8_t> pub) {
		if (pub.empty()) {
			return Result<Identity>(Error::InvalidArg);
		}
		std::uint64_t h = rns_identity_from_public_key(pub.data(), pub.size());
		if (h == 0) {
			return Result<Identity>(Error::InvalidArg);
		}
		return Result<Identity>(Identity(h));
	}

	Result<std::vector<std::uint8_t>> sign(span<const std::uint8_t> data) const {
		std::uint8_t buf[64];
		std::size_t written = 0;
		Error err = map_code(rns_identity_sign(handle_, data.data(), data.size(), buf,
							sizeof(buf), &written));
		if (err != Error::Ok) {
			return Result<std::vector<std::uint8_t>>(err);
		}
		if (written > sizeof(buf)) {
			written = sizeof(buf);
		}
		return Result<std::vector<std::uint8_t>>(
		    std::vector<std::uint8_t>(buf, buf + written));
	}

	Error verify(span<const std::uint8_t> data, span<const std::uint8_t> signature) const {
		return map_code(rns_identity_verify(handle_, data.data(), data.size(),
						    signature.data(), signature.size()));
	}

	std::uint64_t handle() const noexcept { return handle_; }
	explicit operator bool() const noexcept { return handle_ != 0; }

	void release() noexcept { handle_ = 0; }

	void reset() noexcept {
		if (handle_ != 0) {
			rns_identity_destroy(handle_);
			handle_ = 0;
		}
	}

      private:
	explicit Identity(std::uint64_t handle) noexcept : handle_(handle) {}

	std::uint64_t handle_;
};

} // namespace rns

#endif
