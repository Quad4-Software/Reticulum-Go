// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_PATH_HPP
#define RNS_PATH_HPP

#include <cstddef>
#include <cstdint>
#include <cstring>
#include <string>
#include <vector>

#include "rns/detail/c_api.hpp"
#include "rns/error.hpp"
#include "rns/node.hpp"
#include "rns/span.hpp"
#include "rns/types.hpp"
#include "rns/util.hpp"

namespace rns {

struct PathEntry {
	Hash hash{};
	std::size_t hash_size = 0;
	Hash via{};
	std::size_t via_size = 0;
	std::uint8_t hops = 0;
	std::string iface;
	double timestamp = 0;
	double expires = 0;

	static PathEntry from_c(const rns_path_entry &e) {
		PathEntry out;
		out.hash_size = e.hash_len;
		out.via_size = e.via_len;
		out.hops = e.hops;
		out.timestamp = e.timestamp;
		out.expires = e.expires;
		std::memcpy(out.hash.data(), e.hash, rns::hash_len);
		std::memcpy(out.via.data(), e.via, rns::hash_len);
		out.iface = std::string(util::cstring_field(e.iface, sizeof(e.iface)));
		return out;
	}
};

inline Error path_request(Node &node, span<const std::uint8_t> dest_hash) {
	if (dest_hash.size() != hash_len) {
		return Error::InvalidArg;
	}
	return map_code(rns_path_request(node.handle(), dest_hash.data()));
}

inline Error path_request(Node &node, const Hash &dest_hash) {
	return path_request(node, span<const std::uint8_t>(dest_hash.data(), dest_hash.size()));
}

inline Result<std::vector<PathEntry>> path_table(Node &node, std::size_t capacity = 256,
						 int max_hops = -1) {
	if (capacity == 0) {
		return Result<std::vector<PathEntry>>(Error::InvalidArg);
	}
	std::vector<rns_path_entry> raw(capacity);
	std::size_t written = 0;
	Error err =
	    map_code(rns_path_table(node.handle(), raw.data(), raw.size(), &written, max_hops));
	if (err != Error::Ok) {
		return Result<std::vector<PathEntry>>(err);
	}
	std::vector<PathEntry> out;
	out.reserve(written);
	for (std::size_t i = 0; i < written; ++i) {
		out.push_back(PathEntry::from_c(raw[i]));
	}
	return Result<std::vector<PathEntry>>(std::move(out));
}

} // namespace rns

#endif
