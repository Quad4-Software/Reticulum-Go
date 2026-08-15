// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#include <catch2/catch_test_macros.hpp>

#include <rns/rns.hpp>

TEST_CASE("version matches api_version", "[smoke]") {
	REQUIRE(rns::version() == rns::api_version);
}

TEST_CASE("node lifecycle", "[smoke]") {
	auto node_r = rns::Node::create("");
	REQUIRE(node_r.ok());
	auto node = std::move(node_r).value();

	REQUIRE(node.start() == rns::Error::Ok);
	auto poll = node.poll(10);
	REQUIRE_FALSE(poll.ok());
	REQUIRE(poll.error() == rns::Error::Timeout);
	REQUIRE(node.stop() == rns::Error::Ok);
}

TEST_CASE("invalid handle", "[smoke]") {
	rns::Node bad;
	auto err = bad.start();
	REQUIRE((err == rns::Error::InvalidHandle || err == rns::Error::InvalidArg ||
		 err == rns::Error::Internal));
}
