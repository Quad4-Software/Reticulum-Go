// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_EVENT_HPP
#define RNS_EVENT_HPP

#include <cstdint>
#include <cstring>
#include <functional>
#include <string>
#include <string_view>
#include <vector>

#include "rns/detail/c_api.hpp"
#include "rns/error.hpp"
#include "rns/span.hpp"
#include "rns/types.hpp"
#include "rns/util.hpp"

namespace rns {

class Event {
      public:
	Event() = default;

	static Event from_c(const rns_event &ev) {
		Event out;
		out.kind_ = static_cast<EventKind>(ev.kind);
		out.hops_ = ev.hops;
		out.path_truncated_ = ev.path_truncated != 0;
		out.error_message_truncated_ = ev.error_message_truncated != 0;
		out.app_data_truncated_ = ev.app_data_truncated != 0;

		out.link_id_.assign(ev.link_id, ev.link_id + ev.link_id_len);
		out.destination_hash_.assign(ev.destination_hash,
					     ev.destination_hash + ev.destination_hash_len);
		out.identity_hash_.assign(ev.identity_hash,
					  ev.identity_hash + ev.identity_hash_len);
		out.request_id_.assign(ev.request_id, ev.request_id + ev.request_id_len);

		out.path_ = std::string(util::cstring_field(ev.path, sizeof(ev.path)));
		out.error_message_ =
		    std::string(util::cstring_field(ev.error_message, sizeof(ev.error_message)));

		if (ev.app_data != nullptr && ev.app_data_len > 0) {
			out.app_data_.assign(ev.app_data, ev.app_data + ev.app_data_len);
		}
		return out;
	}

	EventKind kind() const noexcept { return kind_; }
	std::uint8_t hops() const noexcept { return hops_; }

	span<const std::uint8_t> link_id() const noexcept {
		return {link_id_.data(), link_id_.size()};
	}
	span<const std::uint8_t> destination_hash() const noexcept {
		return {destination_hash_.data(), destination_hash_.size()};
	}
	span<const std::uint8_t> identity_hash() const noexcept {
		return {identity_hash_.data(), identity_hash_.size()};
	}
	span<const std::uint8_t> request_id() const noexcept {
		return {request_id_.data(), request_id_.size()};
	}
	span<const std::uint8_t> app_data() const noexcept {
		return {app_data_.data(), app_data_.size()};
	}

	std::string_view path() const noexcept { return path_; }
	std::string_view error_message() const noexcept { return error_message_; }

	bool path_truncated() const noexcept { return path_truncated_; }
	bool error_message_truncated() const noexcept { return error_message_truncated_; }
	bool app_data_truncated() const noexcept { return app_data_truncated_; }

      private:
	EventKind kind_ = EventKind::None;
	std::uint8_t hops_ = 0;
	std::vector<std::uint8_t> link_id_;
	std::vector<std::uint8_t> destination_hash_;
	std::vector<std::uint8_t> identity_hash_;
	std::vector<std::uint8_t> request_id_;
	std::vector<std::uint8_t> app_data_;
	std::string path_;
	std::string error_message_;
	bool path_truncated_ = false;
	bool error_message_truncated_ = false;
	bool app_data_truncated_ = false;
};

using EventCallback = std::function<void(const Event &)>;

} // namespace rns

extern "C" void rns_cpp_event_trampoline(const rns_event *event, void *user_data);

namespace rns {
// Installed by Node::set_event_callback.
inline rns_event_callback event_callback_c_fn() noexcept {
	return &rns_cpp_event_trampoline;
}
} // namespace rns

#endif
