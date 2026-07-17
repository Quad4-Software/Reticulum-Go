// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#include <catch2/catch_test_macros.hpp>

#include <rns/rns.hpp>

TEST_CASE("destination create announce hash", "[destination]") {
	auto node_r = rns::Node::create("");
	REQUIRE(node_r.ok());
	auto node = std::move(node_r).value();

	auto id_r = rns::Identity::generate();
	REQUIRE(id_r.ok());
	auto id = std::move(id_r).value();

	REQUIRE(node.set_identity(id) == rns::Error::Ok);
	REQUIRE(node.start() == rns::Error::Ok);

	auto dest_r = rns::Destination::create(node, nullptr, "cpp-rns", {"chat"}, true);
	REQUIRE(dest_r.ok());
	auto dest = std::move(dest_r).value();

	REQUIRE(dest.announce(std::string_view("hello")) == rns::Error::Ok);

	auto hash = dest.hash();
	REQUIRE(hash.ok());
	REQUIRE(hash->size() == rns::hash_len);

	auto table = rns::path_table(node, 8, -1);
	if (!table.ok()) {
		REQUIRE(table.error() == rns::Error::NotFound);
	}

	REQUIRE(node.stop() == rns::Error::Ok);
}

TEST_CASE("destination register request handler", "[destination]") {
	auto node_r = rns::Node::create("");
	REQUIRE(node_r.ok());
	auto node = std::move(node_r).value();

	auto id_r = rns::Identity::generate();
	REQUIRE(id_r.ok());
	auto id = std::move(id_r).value();

	REQUIRE(node.set_identity(id) == rns::Error::Ok);
	REQUIRE(node.start() == rns::Error::Ok);

	auto dest_r = rns::Destination::create(node, nullptr, "cpp-rns", {"page"}, true);
	REQUIRE(dest_r.ok());
	auto dest = std::move(dest_r).value();

	REQUIRE(dest.register_request_handler("/page/index.mu") == rns::Error::Ok);
	REQUIRE(node.stop() == rns::Error::Ok);
}
