// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_INTERFACES_HPP
#define RNS_INTERFACES_HPP

#include <cstddef>
#include <cstdint>
#include <string>
#include <utility>
#include <vector>

#include "rns/detail/c_api.hpp"
#include "rns/error.hpp"
#include "rns/node.hpp"
#include "rns/util.hpp"

namespace rns {

struct InterfaceEntry {
	std::string name;
	std::string type_name;
	bool online = false;
	bool enabled = false;
	std::uint64_t rx_bytes = 0;
	std::uint64_t tx_bytes = 0;
	std::uint64_t rx_packets = 0;
	std::uint64_t tx_packets = 0;

	static InterfaceEntry from_c(const rns_interface_entry &e) {
		InterfaceEntry out;
		out.name = std::string(util::cstring_field(e.name, sizeof(e.name)));
		out.type_name = std::string(util::cstring_field(e.type_name, sizeof(e.type_name)));
		out.online = e.online != 0;
		out.enabled = e.enabled != 0;
		out.rx_bytes = e.rx_bytes;
		out.tx_bytes = e.tx_bytes;
		out.rx_packets = e.rx_packets;
		out.tx_packets = e.tx_packets;
		return out;
	}
};

inline Result<std::vector<InterfaceEntry>> interfaces_list(Node &node, std::size_t capacity = 32) {
	if (capacity == 0) {
		return Result<std::vector<InterfaceEntry>>(Error::InvalidArg);
	}
	std::vector<rns_interface_entry> raw(capacity);
	std::size_t written = 0;
	Error err = map_code(rns_interfaces(node.handle(), raw.data(), raw.size(), &written));
	if (err != Error::Ok && err != Error::Truncated) {
		return Result<std::vector<InterfaceEntry>>(err);
	}
	std::vector<InterfaceEntry> out;
	out.reserve(written);
	for (std::size_t i = 0; i < written; ++i) {
		out.push_back(InterfaceEntry::from_c(raw[i]));
	}
	return Result<std::vector<InterfaceEntry>>(std::move(out));
}

} // namespace rns

#endif
