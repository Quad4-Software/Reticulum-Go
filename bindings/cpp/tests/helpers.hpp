// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_CPP_TEST_HELPERS_HPP
#define RNS_CPP_TEST_HELPERS_HPP

#include <chrono>
#include <cstdio>
#include <cstdlib>
#include <fstream>
#include <sstream>
#include <string>
#include <vector>

#include <arpa/inet.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>

#include <rns/rns.hpp>

namespace rns_test {

inline std::uint16_t free_udp_port() {
	int fd = ::socket(AF_INET, SOCK_DGRAM, 0);
	if (fd < 0) {
		return 0;
	}
	sockaddr_in addr{};
	addr.sin_family = AF_INET;
	addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
	addr.sin_port = htons(0);
	if (::bind(fd, reinterpret_cast<sockaddr *>(&addr), sizeof(addr)) != 0) {
		::close(fd);
		return 0;
	}
	socklen_t len = sizeof(addr);
	if (::getsockname(fd, reinterpret_cast<sockaddr *>(&addr), &len) != 0) {
		::close(fd);
		return 0;
	}
	std::uint16_t port = ntohs(addr.sin_port);
	::close(fd);
	return port;
}

inline std::string make_temp_dir() {
	std::string tmpl = "/tmp/rns_cpp_XXXXXX";
	std::vector<char> buf(tmpl.begin(), tmpl.end());
	buf.push_back('\0');
	char *dir = ::mkdtemp(buf.data());
	if (dir == nullptr) {
		return {};
	}
	return std::string(dir);
}

inline bool write_udp_peer_config(const std::string &dir, std::uint16_t listen,
				  std::uint16_t peer) {
	std::ostringstream oss;
	oss << "[reticulum]\n"
	    << "enable_transport = yes\n"
	    << "share_instance = no\n"
	    << "\n"
	    << "[interfaces]\n"
	    << "  [[UDP]]\n"
	    << "    type = UDPInterface\n"
	    << "    enabled = yes\n"
	    << "    listen_ip = 127.0.0.1\n"
	    << "    listen_port = " << listen << "\n"
	    << "    target_host = 127.0.0.1\n"
	    << "    target_port = " << peer << "\n";
	std::ofstream out(dir + "/config");
	if (!out) {
		return false;
	}
	out << oss.str();
	return static_cast<bool>(out);
}

inline std::string config_path(const std::string &dir) {
	return dir + "/config";
}

inline rns::Result<rns::Event> poll_until(rns::Node &node, rns::EventKind want, int timeout_ms,
					  rns::span<std::uint8_t> app_buf = {}) {
	int remaining = timeout_ms;
	while (remaining > 0) {
		constexpr int step = 50;
		auto ev = node.poll(step, app_buf);
		if (ev.ok()) {
			if (ev->kind() == want) {
				return ev;
			}
		} else if (ev.error() != rns::Error::Timeout) {
			return ev;
		}
		remaining -= step;
	}
	return rns::Result<rns::Event>(rns::Error::Timeout);
}

inline void remove_temp_dir(const std::string &dir) {
	if (dir.empty()) {
		return;
	}
	std::string cmd = "rm -rf \"" + dir + "\"";
	(void)std::system(cmd.c_str());
}

} // namespace rns_test

#endif
