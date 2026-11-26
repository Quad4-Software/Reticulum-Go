// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

#include <catch2/catch_test_macros.hpp>

#include <filesystem>
#include <fstream>

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

TEST_CASE("added abi surface smoke", "[smoke]") {
	auto node_r = rns::Node::create("");
	REQUIRE(node_r.ok());
	auto node = std::move(node_r).value();
	REQUIRE(node.reload_config() == rns::Error::InvalidArg);
	const auto config_path = std::filesystem::temp_directory_path() / "rns-cpp-reload.conf";
	{
		std::ofstream out(config_path);
		out << "[reticulum]\nenable_transport = no\nin_memory_storage = yes\n";
	}
	auto configured_r = rns::Node::create(config_path.string());
	REQUIRE(configured_r.ok());
	auto configured = std::move(configured_r).value();
	REQUIRE(configured.reload_config() == rns::Error::Ok);
	std::filesystem::remove(config_path);
	rns::Hash unknown{};
	auto encrypted = rns::destination_encrypt(unknown, rns::span<const std::uint8_t>(
	    reinterpret_cast<const std::uint8_t *>("x"), 1));
	REQUIRE_FALSE(encrypted.ok());
	REQUIRE(encrypted.error() == rns::Error::NotFound);
	REQUIRE(node.packet_send(unknown, rns::span<const std::uint8_t>(
	    reinterpret_cast<const std::uint8_t *>("x"), 1)) == rns::Error::State);
	auto missing = rns::Link::from_id(node, rns::span<const std::uint8_t>(unknown.data(), unknown.size()));
	REQUIRE_FALSE(missing.ok());
	REQUIRE(missing.error() == rns::Error::NotFound);

	auto identity_r = rns::Identity::generate();
	REQUIRE(identity_r.ok());
	auto identity = std::move(identity_r).value();
	auto dest_r = rns::Destination::create(node, &identity, "cpptest", {}, false);
	REQUIRE(dest_r.ok());
	auto dest = std::move(dest_r).value();
	REQUIRE(dest.set_proof_strategy(0) == rns::Error::Ok);
	REQUIRE(dest.set_proof_strategy(255) == rns::Error::InvalidArg);
}
