// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_TYPES_HPP
#define RNS_TYPES_HPP

#include <array>
#include <cstddef>
#include <cstdint>

#include "rns/detail/c_api.hpp"

namespace rns {

constexpr const char *api_version = RNS_API_VERSION;
constexpr std::size_t hash_len = RNS_HASH_LEN;

using Hash = std::array<std::uint8_t, hash_len>;

enum class EventKind : int {
	None = 0,
	Announce = RNS_EV_ANNOUNCE,
	LinkEstablished = RNS_EV_LINK_ESTABLISHED,
	LinkFailed = RNS_EV_LINK_FAILED,
	LinkData = RNS_EV_LINK_DATA,
	LinkClosed = RNS_EV_LINK_CLOSED,
	RequestIncoming = RNS_EV_REQUEST_INCOMING,
	RequestResponse = RNS_EV_REQUEST_RESPONSE,
	RequestFailed = RNS_EV_REQUEST_FAILED,
	ResourceStarted = RNS_EV_RESOURCE_STARTED,
	ResourceConcluded = RNS_EV_RESOURCE_CONCLUDED,
	DestinationData = RNS_EV_DESTINATION_DATA,
};

} // namespace rns

#endif
