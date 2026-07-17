// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_DESTINATION_HPP
#define RNS_DESTINATION_HPP

#include <cstdint>
#include <initializer_list>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

#include "rns/detail/c_api.hpp"
#include "rns/error.hpp"
#include "rns/identity.hpp"
#include "rns/node.hpp"
#include "rns/span.hpp"
#include "rns/types.hpp"
#include "rns/util.hpp"

namespace rns {

class Destination {
      public:
	Destination() noexcept : handle_(0) {}

	Destination(Destination &&other) noexcept : handle_(other.handle_) { other.handle_ = 0; }

	Destination &operator=(Destination &&other) noexcept {
		if (this != &other) {
			reset();
			handle_ = other.handle_;
			other.handle_ = 0;
		}
		return *this;
	}

	Destination(const Destination &) = delete;
	Destination &operator=(const Destination &) = delete;

	~Destination() { reset(); }

	// identity may be nullptr to use the node identity (handle 0).
	static Result<Destination> create(Node &node, Identity *identity, std::string_view app_name,
					  span<const std::string_view> aspects,
					  bool accepts_links) {

		if (app_name.empty()) {
			return Result<Destination>(Error::InvalidArg);
		}

		char app_buf[256];
		const char *app_z = nullptr;
		Error cerr = util::require_cstring(app_name, app_buf, sizeof(app_buf), &app_z);
		if (cerr != Error::Ok) {
			return Result<Destination>(cerr);
		}

		constexpr std::size_t kMaxAspects = 8;
		if (aspects.size() > kMaxAspects) {
			return Result<Destination>(Error::InvalidArg);
		}

		char aspect_storage[kMaxAspects][128];
		const char *aspect_ptrs[kMaxAspects];
		for (std::size_t i = 0; i < aspects.size(); ++i) {
			const char *az = nullptr;
			cerr = util::require_cstring(aspects[i], aspect_storage[i],
						     sizeof(aspect_storage[i]), &az);
			if (cerr != Error::Ok) {
				return Result<Destination>(cerr);
			}
			aspect_ptrs[i] = az;
		}

		std::uint64_t id_handle = identity ? identity->handle() : 0;
		std::uint64_t h = rns_destination_create(node.handle(), id_handle, app_z,
							 aspects.empty() ? nullptr : aspect_ptrs,
							 aspects.size(), accepts_links ? 1 : 0);
		if (h == 0) {
			return Result<Destination>(Error::Internal);
		}
		return Result<Destination>(Destination(h));
	}

	static Result<Destination> create(Node &node, Identity *identity, std::string_view app_name,
					  std::initializer_list<std::string_view> aspects,
					  bool accepts_links) {
		return create(node, identity, app_name,
			      span<const std::string_view>(aspects.begin(), aspects.size()),
			      accepts_links);
	}

	Error announce(const std::uint8_t *data, std::size_t len) {
		if (data == nullptr || len == 0) {
			return map_code(rns_destination_announce(handle_, nullptr, 0));
		}
		return map_code(rns_destination_announce(handle_, data, len));
	}

	Error announce() { return announce(nullptr, 0); }

	Error announce(std::string_view app_data) {
		return announce(reinterpret_cast<const std::uint8_t *>(app_data.data()),
				app_data.size());
	}

	Error announce(span<const std::uint8_t> app_data) {
		return announce(app_data.data(), app_data.size());
	}

	Result<Hash> hash() const {
		Hash out{};
		std::size_t written = 0;
		Error err =
		    map_code(rns_destination_hash(handle_, out.data(), out.size(), &written));
		if (err != Error::Ok) {
			return Result<Hash>(err);
		}
		if (written != hash_len) {
			return Result<Hash>(Error::Truncated);
		}
		return Result<Hash>(out);
	}

	Error register_request_handler(std::string_view path) {
		char path_buf[256];
		const char *path_z = nullptr;
		Error cerr = util::require_cstring(path, path_buf, sizeof(path_buf), &path_z);
		if (cerr != Error::Ok) {
			return cerr;
		}
		return map_code(rns_destination_register_request_handler(handle_, path_z));
	}

	std::uint64_t handle() const noexcept { return handle_; }
	explicit operator bool() const noexcept { return handle_ != 0; }

	void reset() noexcept {
		if (handle_ != 0) {
			rns_destination_destroy(handle_);
			handle_ = 0;
		}
	}

      private:
	explicit Destination(std::uint64_t handle) noexcept : handle_(handle) {}

	std::uint64_t handle_;
};

} // namespace rns

#endif
