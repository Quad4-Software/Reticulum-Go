// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_NODE_HPP
#define RNS_NODE_HPP

#include <cstdint>
#include <cstring>
#include <memory>
#include <string_view>
#include <utility>
#include <vector>

#include "rns/detail/c_api.hpp"
#include "rns/error.hpp"
#include "rns/event.hpp"
#include "rns/identity.hpp"
#include "rns/span.hpp"
#include "rns/types.hpp"
#include "rns/util.hpp"

namespace rns {

class Node {
      public:
	Node() noexcept : handle_(0) {}

	Node(Node &&other) noexcept
	    : handle_(other.handle_), callback_(std::move(other.callback_)) {
		other.handle_ = 0;
	}

	Node &operator=(Node &&other) noexcept {
		if (this != &other) {
			reset();
			handle_ = other.handle_;
			callback_ = std::move(other.callback_);
			other.handle_ = 0;
		}
		return *this;
	}

	Node(const Node &) = delete;
	Node &operator=(const Node &) = delete;

	~Node() { reset(); }

	static Result<Node> create(std::string_view config_path = {}) {
		char path_buf[4096];
		const char *path_z = "";
		if (!config_path.empty()) {
			Error cerr =
			    util::require_cstring(config_path, path_buf, sizeof(path_buf), &path_z);
			if (cerr != Error::Ok) {
				return Result<Node>(cerr);
			}
		}
		std::uint64_t h = rns_node_create(path_z);
		if (h == 0) {
			return Result<Node>(Error::Internal);
		}
		return Result<Node>(Node(h));
	}

	Error start() { return map_code(rns_node_start(handle_)); }

	Error stop() { return map_code(rns_node_stop(handle_)); }

	Error pause() { return map_code(rns_node_pause(handle_)); }

	Error resume() { return map_code(rns_node_resume(handle_)); }

	Error set_identity(Identity &identity) {
		return map_code(rns_node_set_identity(handle_, identity.handle()));
	}

	Error refresh_paths() { return map_code(rns_node_refresh_paths(handle_, nullptr, 0)); }

	Error refresh_paths(span<const Hash> dest_hashes) {
		if (dest_hashes.empty()) {
			return refresh_paths();
		}
		std::vector<std::uint8_t> flat(dest_hashes.size() * hash_len);
		for (std::size_t i = 0; i < dest_hashes.size(); ++i) {
			std::memcpy(flat.data() + i * hash_len, dest_hashes[i].data(), hash_len);
		}
		return map_code(rns_node_refresh_paths(handle_, flat.data(), dest_hashes.size()));
	}

	Result<Event> poll(int timeout_ms, span<std::uint8_t> app_data_buf = {}) {
		rns_event ev;
		std::memset(&ev, 0, sizeof(ev));
		if (!app_data_buf.empty()) {
			ev.app_data = app_data_buf.data();
			ev.app_data_cap = app_data_buf.size();
		}
		Error err = map_code(rns_event_poll(handle_, &ev, timeout_ms));
		if (err != Error::Ok) {
			return Result<Event>(err);
		}
		return Result<Event>(Event::from_c(ev));
	}

	Error set_event_callback(EventCallback callback) {
		if (!callback) {
			Error err = map_code(rns_set_event_callback(handle_, nullptr, nullptr));
			callback_.reset();
			return err;
		}
		callback_ = std::make_unique<EventCallback>(std::move(callback));
		return map_code(
		    rns_set_event_callback(handle_, event_callback_c_fn(), callback_.get()));
	}

	std::uint64_t handle() const noexcept { return handle_; }
	explicit operator bool() const noexcept { return handle_ != 0; }

	void reset() noexcept {
		if (handle_ != 0) {
			rns_set_event_callback(handle_, nullptr, nullptr);
			rns_node_destroy(handle_);
			handle_ = 0;
		}
		callback_.reset();
	}

      private:
	explicit Node(std::uint64_t handle) noexcept : handle_(handle) {}

	std::uint64_t handle_;
	std::unique_ptr<EventCallback> callback_;
};

} // namespace rns

#endif
