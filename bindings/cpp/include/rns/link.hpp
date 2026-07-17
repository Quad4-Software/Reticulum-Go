// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_LINK_HPP
#define RNS_LINK_HPP

#include <cstdint>
#include <string_view>
#include <utility>

#include "rns/detail/c_api.hpp"
#include "rns/error.hpp"
#include "rns/node.hpp"
#include "rns/span.hpp"
#include "rns/types.hpp"
#include "rns/util.hpp"

namespace rns {

class Link {
      public:
	Link() noexcept : handle_(0) {}

	Link(Link &&other) noexcept : handle_(other.handle_) { other.handle_ = 0; }

	Link &operator=(Link &&other) noexcept {
		if (this != &other) {
			reset();
			handle_ = other.handle_;
			other.handle_ = 0;
		}
		return *this;
	}

	Link(const Link &) = delete;
	Link &operator=(const Link &) = delete;

	~Link() { reset(); }

	static Result<Link> open(Node &node, span<const std::uint8_t> dest_hash) {
		if (dest_hash.size() != hash_len) {
			return Result<Link>(Error::InvalidArg);
		}
		std::uint64_t h = rns_link_open(node.handle(), dest_hash.data());
		if (h == 0) {
			return Result<Link>(Error::Internal);
		}
		return Result<Link>(Link(h));
	}

	static Result<Link> open(Node &node, const Hash &dest_hash) {
		return open(node, span<const std::uint8_t>(dest_hash.data(), dest_hash.size()));
	}

	Error send(const std::uint8_t *data, std::size_t len) {
		if (data == nullptr || len == 0) {
			return Error::InvalidArg;
		}
		return map_code(rns_link_send(handle_, data, len));
	}

	Error send(std::string_view data) {
		return send(reinterpret_cast<const std::uint8_t *>(data.data()), data.size());
	}

	Error send(span<const std::uint8_t> data) { return send(data.data(), data.size()); }

	Error send_resource(span<const std::uint8_t> data, std::string_view name = {}) {
		char name_buf[256];
		const char *name_z = nullptr;
		if (!name.empty()) {
			Error cerr =
			    util::require_cstring(name, name_buf, sizeof(name_buf), &name_z);
			if (cerr != Error::Ok) {
				return cerr;
			}
		}
		return map_code(rns_link_send_resource(
		    handle_, data.empty() ? nullptr : data.data(), data.size(), name_z));
	}

	Error close() {
		Error err = map_code(rns_link_close(handle_));
		if (err == Error::Ok) {
			handle_ = 0;
		}
		return err;
	}

	Result<Hash> id() const {
		Hash out{};
		std::size_t written = 0;
		Error err = map_code(rns_link_id(handle_, out.data(), out.size(), &written));
		if (err != Error::Ok) {
			return Result<Hash>(err);
		}
		if (written != hash_len) {
			return Result<Hash>(Error::Truncated);
		}
		return Result<Hash>(out);
	}

	Result<Hash> request(Node &node, std::string_view path, span<const std::uint8_t> data,
			     int timeout_ms) {
		char path_buf[256];
		const char *path_z = nullptr;
		Error cerr = util::require_cstring(path, path_buf, sizeof(path_buf), &path_z);
		if (cerr != Error::Ok) {
			return Result<Hash>(cerr);
		}
		Hash request_id{};
		std::size_t written = 0;
		Error err = map_code(rns_link_request(
		    node.handle(), handle_, path_z, data.empty() ? nullptr : data.data(),
		    data.size(), timeout_ms, request_id.data(), request_id.size(), &written));
		if (err != Error::Ok) {
			return Result<Hash>(err);
		}
		if (written != hash_len) {
			return Result<Hash>(Error::Truncated);
		}
		return Result<Hash>(request_id);
	}

	Result<Hash> request(Node &node, std::string_view path, int timeout_ms) {
		return request(node, path, {}, timeout_ms);
	}

	std::uint64_t handle() const noexcept { return handle_; }
	explicit operator bool() const noexcept { return handle_ != 0; }

	void release() noexcept { handle_ = 0; }

	void reset() noexcept {
		if (handle_ != 0) {
			rns_link_close(handle_);
			handle_ = 0;
		}
	}

      private:
	explicit Link(std::uint64_t handle) noexcept : handle_(handle) {}

	std::uint64_t handle_;
};

inline Error request_respond(Node &node, span<const std::uint8_t> request_id,
			     span<const std::uint8_t> data) {
	if (request_id.empty()) {
		return Error::InvalidArg;
	}
	return map_code(rns_request_respond(node.handle(), request_id.data(), request_id.size(),
					    data.empty() ? nullptr : data.data(), data.size()));
}

inline Error request_respond(Node &node, span<const std::uint8_t> request_id,
			     std::string_view data) {
	return request_respond(
	    node, request_id,
	    span<const std::uint8_t>(reinterpret_cast<const std::uint8_t *>(data.data()),
				     data.size()));
}

inline Error request_respond_file(Node &node, span<const std::uint8_t> request_id,
				  std::string_view filename, span<const std::uint8_t> data) {
	if (request_id.empty() || filename.empty()) {
		return Error::InvalidArg;
	}
	char name_buf[256];
	const char *name_z = nullptr;
	Error cerr = util::require_cstring(filename, name_buf, sizeof(name_buf), &name_z);
	if (cerr != Error::Ok) {
		return cerr;
	}
	return map_code(
	    rns_request_respond_file(node.handle(), request_id.data(), request_id.size(), name_z,
				     data.empty() ? nullptr : data.data(), data.size()));
}

} // namespace rns

#endif
