// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#include <cstdio>
#include <cstring>
#include <string>

#include <rns/rns.hpp>

int main() {
  std::string ver = rns::version();
  if (ver != rns::api_version) {
    std::fprintf(stderr, "unexpected version: %s\n",
                 ver.empty() ? "(null)" : ver.c_str());
    return 1;
  }

  auto node_r = rns::Node::create("");
  if (!node_r.ok()) {
    std::fprintf(stderr, "rns::Node::create failed: %s\n",
                 rns::last_error().c_str());
    return 1;
  }
  auto node = std::move(node_r).value();

  if (node.start() != rns::Error::Ok) {
    std::fprintf(stderr, "node.start failed: %s\n", rns::last_error().c_str());
    return 1;
  }

  auto poll = node.poll(10);
  if (poll.ok() || poll.error() != rns::Error::Timeout) {
    std::fprintf(stderr, "expected timeout poll on idle node\n");
    node.stop();
    return 1;
  }

  if (node.stop() != rns::Error::Ok) {
    std::fprintf(stderr, "teardown failed\n");
    return 1;
  }

  std::printf("cpp-smoke ok\n");
  return 0;
}
