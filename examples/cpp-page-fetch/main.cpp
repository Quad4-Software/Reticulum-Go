// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style page fetch over the C++ librns bindings.
// Usage: cpp-page-fetch [-c config] [-t timeout_sec] <dest_hash>:<page_path>

#include <chrono>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>

#include <rns/rns.hpp>

namespace {

constexpr std::size_t kPageBufCap = 512 * 1024;
constexpr int kDefaultTimeoutSec = 60;
constexpr auto kPathRetry = std::chrono::seconds(2);

void usage(const char *argv0) {
  std::fprintf(
      stderr,
      "Usage: %s [-c config] [-t timeout_sec] <dest_hash>:<page_path>\n"
      "\n"
      "Fetch a NomadNet / pageserver page over librns (C++ bindings).\n"
      "\n"
      "Options:\n"
      "  -c path   Reticulum config file (required for network interfaces)\n"
      "  -t sec    Overall timeout in seconds (default %d)\n"
      "\n"
      "Example:\n"
      "  %s -c ../librns-page-fetch/config.example "
      "92798ea245a0afcfa559348e42d628c6:/page/index.mu\n",
      argv0, kDefaultTimeoutSec, argv0);
}

void print_last_error(const char *what) {
  std::string msg = rns::last_error();
  if (!msg.empty()) {
    std::fprintf(stderr, "%s: %s\n", what, msg.c_str());
  } else {
    std::fprintf(stderr, "%s\n", what);
  }
}

bool hash_eq(rns::span<const std::uint8_t> a, const rns::Hash &b) {
  return a.size() == rns::hash_len &&
         std::memcmp(a.data(), b.data(), rns::hash_len) == 0;
}

bool path_known(rns::Node &node, const rns::Hash &dest) {
  auto table = rns::path_table(node);
  if (!table.ok()) {
    return false;
  }
  for (const auto &entry : *table) {
    if (entry.hash_size == rns::hash_len &&
        std::memcmp(entry.hash.data(), dest.data(), rns::hash_len) == 0) {
      return true;
    }
  }
  return false;
}

bool parse_target(const std::string &target, rns::Hash &hash,
                  std::string &page_path) {
  auto colon = target.find(':');
  if (colon == std::string::npos || colon == 0 || colon + 1 >= target.size()) {
    return false;
  }
  auto decoded = rns::util::hex_to_hash(target.substr(0, colon));
  if (!decoded.ok()) {
    return false;
  }
  hash = *decoded;
  page_path = target.substr(colon + 1);
  return true;
}

int run(const std::string &config_path, const std::string &target,
        int timeout_sec) {
  std::string ver = rns::version();
  if (ver != rns::api_version) {
    std::fprintf(stderr, "librns version mismatch: got %s want %s\n",
                 ver.c_str(), rns::api_version);
    return 1;
  }

  rns::Hash dest_hash{};
  std::string page_path;
  if (!parse_target(target, dest_hash, page_path)) {
    std::fprintf(stderr, "target must be <32-hex-dest>:<page_path>\n");
    return 1;
  }
  auto dest_hex_r = rns::util::hash_to_hex(dest_hash);
  if (!dest_hex_r.ok()) {
    std::fprintf(stderr, "failed to encode destination hash\n");
    return 1;
  }
  const std::string &dest_hex = *dest_hex_r;

  auto node_r = rns::Node::create(config_path);
  if (!node_r.ok()) {
    print_last_error("rns::Node::create failed");
    return 1;
  }
  auto node = std::move(node_r).value();

  auto id_r = rns::Identity::generate();
  if (!id_r.ok()) {
    print_last_error("rns::Identity::generate failed");
    return 1;
  }
  auto identity = std::move(id_r).value();

  if (node.set_identity(identity) != rns::Error::Ok) {
    print_last_error("node.set_identity failed");
    return 1;
  }
  if (node.start() != rns::Error::Ok) {
    print_last_error("node.start failed");
    return 1;
  }

  std::printf("librns %s fetching %s from %s\n", ver.c_str(), page_path.c_str(),
              dest_hex.c_str());

  std::vector<std::uint8_t> page_buf(kPageBufCap);
  rns::span<std::uint8_t> page_span(page_buf.data(), page_buf.size());

  auto deadline =
      std::chrono::steady_clock::now() + std::chrono::seconds(timeout_sec);
  auto last_path_req = std::chrono::steady_clock::now();
  bool need_path_req = true;
  bool saw_announce = false;
  rns::Link link;

  while (std::chrono::steady_clock::now() < deadline && !link) {
    auto now = std::chrono::steady_clock::now();
    if (need_path_req || now - last_path_req >= kPathRetry) {
      if (rns::path_request(node, dest_hash) != rns::Error::Ok) {
        print_last_error("path_request failed");
      }
      last_path_req = now;
      need_path_req = false;
      if (path_known(node, dest_hash)) {
        std::fprintf(stderr,
                     "path known, waiting for destination identity announce\n");
      } else {
        std::fprintf(stderr, "requesting path to %s\n", dest_hex.c_str());
      }
    }

    auto ev = node.poll(200, page_span);
    if (!ev.ok()) {
      if (ev.error() == rns::Error::Timeout) {
        if (saw_announce || path_known(node, dest_hash)) {
          auto opened = rns::Link::open(node, dest_hash);
          if (opened.ok()) {
            link = std::move(opened).value();
          }
        }
        continue;
      }
      print_last_error("node.poll failed");
      return 1;
    }

    if (ev->kind() == rns::EventKind::Announce &&
        hash_eq(ev->destination_hash(), dest_hash)) {
      saw_announce = true;
      std::fprintf(stderr, "announce for target (hops=%u)\n",
                   static_cast<unsigned>(ev->hops()));
      auto opened = rns::Link::open(node, dest_hash);
      if (!opened.ok()) {
        print_last_error("Link::open after announce");
      } else {
        link = std::move(opened).value();
      }
    } else if (ev->kind() == rns::EventKind::LinkFailed) {
      std::fprintf(stderr, "link failed while opening: %.*s\n",
                   static_cast<int>(ev->error_message().size()),
                   ev->error_message().data());
    }
  }

  if (!link) {
    std::fprintf(stderr, "timed out before link open\n");
    return 1;
  }

  bool established = false;
  while (std::chrono::steady_clock::now() < deadline && !established) {
    auto ev = node.poll(500, page_span);
    if (!ev.ok()) {
      if (ev.error() == rns::Error::Timeout) {
        continue;
      }
      print_last_error("node.poll failed");
      return 1;
    }
    if (ev->kind() == rns::EventKind::LinkEstablished) {
      established = true;
      std::fprintf(stderr, "link established\n");
    } else if (ev->kind() == rns::EventKind::LinkFailed) {
      std::fprintf(stderr, "link establishment failed: %.*s\n",
                   static_cast<int>(ev->error_message().size()),
                   ev->error_message().data());
      return 1;
    } else if (ev->kind() == rns::EventKind::LinkClosed) {
      std::fprintf(stderr, "link closed before establish\n");
      return 1;
    }
  }

  if (!established) {
    std::fprintf(stderr, "timed out waiting for link establishment\n");
    return 1;
  }

  auto remaining = std::chrono::duration_cast<std::chrono::milliseconds>(
                       deadline - std::chrono::steady_clock::now())
                       .count();
  int timeout_ms = static_cast<int>(remaining);
  if (timeout_ms < 1000) {
    timeout_ms = 1000;
  }

  auto req = link.request(node, page_path, timeout_ms);
  if (!req.ok()) {
    print_last_error("link.request failed");
    return 1;
  }
  std::fprintf(stderr, "request sent for %s\n", page_path.c_str());

  while (std::chrono::steady_clock::now() < deadline) {
    auto ev = node.poll(500, page_span);
    if (!ev.ok()) {
      if (ev.error() == rns::Error::Timeout) {
        continue;
      }
      print_last_error("node.poll failed");
      return 1;
    }

    if (ev->kind() == rns::EventKind::RequestResponse) {
      auto data = ev->app_data();
      std::printf("\n=== Page Content (%zu bytes) ===\n", data.size());
      if (!data.empty()) {
        std::fwrite(data.data(), 1, data.size(), stdout);
        if (data[data.size() - 1] != '\n') {
          std::printf("\n");
        }
      }
      if (ev->app_data_truncated()) {
        std::fprintf(stderr, "warning: response truncated to %zu bytes\n",
                     kPageBufCap);
      }
      std::printf("=== End of Page ===\n");
      return 0;
    }
    if (ev->kind() == rns::EventKind::RequestFailed) {
      std::fprintf(stderr, "request failed: %.*s\n",
                   static_cast<int>(ev->error_message().size()),
                   ev->error_message().data());
      return 1;
    }
    if (ev->kind() == rns::EventKind::LinkClosed) {
      std::fprintf(stderr, "link closed before response\n");
      return 1;
    }
  }

  std::fprintf(stderr, "timed out waiting for page response\n");
  return 1;
}

} // namespace

int main(int argc, char **argv) {
  std::string config_path;
  int timeout_sec = kDefaultTimeoutSec;
  std::string target;

  for (int i = 1; i < argc; ++i) {
    std::string arg = argv[i];
    if (arg == "-c" && i + 1 < argc) {
      config_path = argv[++i];
      continue;
    }
    if (arg == "-t" && i + 1 < argc) {
      timeout_sec = std::atoi(argv[++i]);
      if (timeout_sec <= 0) {
        std::fprintf(stderr, "timeout must be positive\n");
        return 1;
      }
      continue;
    }
    if (arg == "-h" || arg == "--help") {
      usage(argv[0]);
      return 0;
    }
    if (!arg.empty() && arg[0] == '-') {
      std::fprintf(stderr, "unknown option: %s\n", arg.c_str());
      usage(argv[0]);
      return 1;
    }
    if (!target.empty()) {
      std::fprintf(stderr, "extra argument: %s\n", arg.c_str());
      usage(argv[0]);
      return 1;
    }
    target = arg;
  }

  if (target.empty() || config_path.empty()) {
    usage(argv[0]);
    return 1;
  }

  return run(config_path, target, timeout_sec);
}
