// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#include <catch2/catch_test_macros.hpp>

#include <cstring>
#include <string>
#include <vector>

#include <rns/rns.hpp>

#include "helpers.hpp"

TEST_CASE("link announce open send over udp", "[link][interop]") {
	auto port_a = rns_test::free_udp_port();
	auto port_b = rns_test::free_udp_port();
	REQUIRE(port_a != 0);
	REQUIRE(port_b != 0);

	auto dir_a = rns_test::make_temp_dir();
	auto dir_b = rns_test::make_temp_dir();
	REQUIRE_FALSE(dir_a.empty());
	REQUIRE_FALSE(dir_b.empty());
	struct Cleanup {
		std::string a;
		std::string b;
		~Cleanup() {
			rns_test::remove_temp_dir(a);
			rns_test::remove_temp_dir(b);
		}
	} cleanup{dir_a, dir_b};

	REQUIRE(rns_test::write_udp_peer_config(dir_a, port_a, port_b));
	REQUIRE(rns_test::write_udp_peer_config(dir_b, port_b, port_a));

	auto node_a_r = rns::Node::create(rns_test::config_path(dir_a));
	REQUIRE(node_a_r.ok());
	auto node_a = std::move(node_a_r).value();

	auto node_b_r = rns::Node::create(rns_test::config_path(dir_b));
	REQUIRE(node_b_r.ok());
	auto node_b = std::move(node_b_r).value();

	auto id_a_r = rns::Identity::generate();
	REQUIRE(id_a_r.ok());
	auto id_a = std::move(id_a_r).value();

	auto id_b_r = rns::Identity::generate();
	REQUIRE(id_b_r.ok());
	auto id_b = std::move(id_b_r).value();

	REQUIRE(node_a.set_identity(id_a) == rns::Error::Ok);
	REQUIRE(node_b.set_identity(id_b) == rns::Error::Ok);
	REQUIRE(node_a.start() == rns::Error::Ok);
	REQUIRE(node_b.start() == rns::Error::Ok);

	auto dest_a_r = rns::Destination::create(node_a, nullptr, "cpp-rns", {"link"}, true);
	REQUIRE(dest_a_r.ok());
	auto dest_a = std::move(dest_a_r).value();

	const char *app_payload = "cpp-app";
	REQUIRE(dest_a.announce(std::string_view(app_payload)) == rns::Error::Ok);

	std::vector<std::uint8_t> app_buf(256);
	auto announce =
	    rns_test::poll_until(node_b, rns::EventKind::Announce, 5000,
				 rns::span<std::uint8_t>(app_buf.data(), app_buf.size()));
	REQUIRE(announce.ok());
	REQUIRE(std::string(reinterpret_cast<const char *>(announce->app_data().data()),
			    announce->app_data().size()) == app_payload);

	auto dest_hash = dest_a.hash();
	REQUIRE(dest_hash.ok());
	REQUIRE(announce->destination_hash().size() == rns::hash_len);
	REQUIRE(std::memcmp(announce->destination_hash().data(), dest_hash->data(),
			    rns::hash_len) == 0);

	auto link_b_r = rns::Link::open(node_b, *dest_hash);
	REQUIRE(link_b_r.ok());
	auto link_b = std::move(link_b_r).value();

	REQUIRE(rns_test::poll_until(node_b, rns::EventKind::LinkEstablished, 5000).ok());
	REQUIRE(rns_test::poll_until(node_a, rns::EventKind::LinkEstablished, 5000).ok());

	const char *payload = "cpp-link-payload";
	REQUIRE(link_b.send(std::string_view(payload)) == rns::Error::Ok);

	std::vector<std::uint8_t> data_buf(256);
	auto data_ev =
	    rns_test::poll_until(node_a, rns::EventKind::LinkData, 5000,
				 rns::span<std::uint8_t>(data_buf.data(), data_buf.size()));
	REQUIRE(data_ev.ok());
	REQUIRE(std::string(reinterpret_cast<const char *>(data_ev->app_data().data()),
			    data_ev->app_data().size()) == payload);

	REQUIRE(link_b.close() == rns::Error::Ok);
	REQUIRE(node_a.stop() == rns::Error::Ok);
	REQUIRE(node_b.stop() == rns::Error::Ok);
}
