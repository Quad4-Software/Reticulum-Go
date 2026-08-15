// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#include <catch2/catch_test_macros.hpp>

#include <rns/rns.hpp>

#include "helpers.hpp"

TEST_CASE("identity generate hash destroy", "[identity]") {
	auto id_r = rns::Identity::generate();
	REQUIRE(id_r.ok());
	auto id = std::move(id_r).value();

	auto hex = id.hash();
	REQUIRE(hex.ok());
	REQUIRE(hex->size() == 32);

	auto node_r = rns::Node::create("");
	REQUIRE(node_r.ok());
	auto node = std::move(node_r).value();
	REQUIRE(node.set_identity(id) == rns::Error::Ok);
}

TEST_CASE("identity load empty path", "[identity]") {
	auto id = rns::Identity::load("");
	REQUIRE_FALSE(id.ok());
	REQUIRE(id.error() == rns::Error::InvalidArg);
}

TEST_CASE("identity save load round trip", "[identity]") {
	auto dir = rns_test::make_temp_dir();
	REQUIRE_FALSE(dir.empty());
	struct Cleanup {
		std::string path;
		~Cleanup() { rns_test::remove_temp_dir(path); }
	} cleanup{dir};

	auto id_r = rns::Identity::generate();
	REQUIRE(id_r.ok());
	auto id = std::move(id_r).value();
	auto hex1 = id.hash();
	REQUIRE(hex1.ok());

	std::string path = dir + "/identity";
	REQUIRE(id.save(path) == rns::Error::Ok);

	auto loaded_r = rns::Identity::load(path);
	REQUIRE(loaded_r.ok());
	auto loaded = std::move(loaded_r).value();
	auto hex2 = loaded.hash();
	REQUIRE(hex2.ok());
	REQUIRE(*hex1 == *hex2);
}
