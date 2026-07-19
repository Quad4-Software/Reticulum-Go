// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style pageserver over the C++ librns bindings.
// Usage: cpp-pageserver -c config [-i identity] [-a announce_sec] [-p
// page_file] [-P request_path]

#include <chrono>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fstream>
#include <sstream>
#include <string>
#include <vector>

#include <rns/rns.hpp>

namespace {

constexpr int kDefaultAnnounceSec = 900;
constexpr const char *kDefaultPagePath = "/page/index.mu";
constexpr const char *kDefaultFilePath = "/file/test.txt";
constexpr const char *kDefaultPageFile = "pages/index.mu";
constexpr const char *kDefaultFileFile = "files/test.txt";
constexpr const char *kDefaultIdentityPath = "identity";
constexpr std::size_t kReqDataCap = 64 * 1024;

constexpr const char *kFallbackPage =
    "> C++ pageserver\n\n"
    "librns via Reticulum-Go\n\n"
    "Fallback page (file not found).\n\n"
    "`[Home`:/page/index.mu]\n"
    "`[Download Test File`:/file/test.txt]`_`f\n\n"
    "---\n";

constexpr const char *kFallbackFile = "Test file from Reticulum-Go node!\n";

void usage(const char *argv0) {
  std::fprintf(
      stderr,
      "Usage: %s -c config [-i identity] [-a announce_sec] [-p page_file] [-f "
      "file] [-P request_path]\n"
      "\n"
      "Serve a NomadNet-compatible /page/ handler over librns.\n"
      "Destination: nomadnetwork.node\n"
      "Announce app_data name: librns-cpp-pageserver\n"
      "\n"
      "Options:\n"
      "  -c path   Reticulum config file (required)\n"
      "  -i path   Persistent identity file (default %s)\n"
      "            Loaded when present, otherwise generated and saved\n"
      "  -a sec    Announce interval seconds (default %d, 0 = once)\n"
      "  -p file   Micron page file to serve (default %s)\n"
      "  -f file   Download file to serve at /file/test.txt (default %s)\n"
      "  -P path   Request path to register (default %s)\n",
      argv0, kDefaultIdentityPath, kDefaultAnnounceSec, kDefaultPageFile,
      kDefaultFileFile, kDefaultPagePath);
}

void print_last_error(const char *what) {
  std::string msg = rns::last_error();
  if (!msg.empty()) {
    std::fprintf(stderr, "%s: %s\n", what, msg.c_str());
  } else {
    std::fprintf(stderr, "%s\n", what);
  }
}

std::vector<std::uint8_t> load_bytes(const std::string &path,
                                     const char *fallback) {
  std::ifstream in(path, std::ios::binary);
  if (!in) {
    std::fprintf(stderr, "warning: could not read %s, using built-in content\n",
                 path.c_str());
    return std::vector<std::uint8_t>(fallback,
                                     fallback + std::strlen(fallback));
  }
  std::ostringstream oss;
  oss << in.rdbuf();
  std::string data = oss.str();
  return std::vector<std::uint8_t>(data.begin(), data.end());
}

rns::Result<rns::Identity> load_or_create_identity(const std::string &path) {
  auto loaded = rns::Identity::load(path);
  if (loaded.ok()) {
    std::fprintf(stderr, "loaded identity from %s\n", path.c_str());
    return loaded;
  }
  auto generated = rns::Identity::generate();
  if (!generated.ok()) {
    return generated;
  }
  if (generated->save(path) != rns::Error::Ok) {
    return rns::Result<rns::Identity>(rns::Error::Io);
  }
  std::fprintf(stderr, "created and saved identity to %s\n", path.c_str());
  return generated;
}

int run(const std::string &config_path, const std::string &identity_path,
        const std::string &page_file, const std::string &file_file,
        const std::string &request_path, const std::string &file_path,
        int announce_sec) {
  std::string ver = rns::version();
  if (ver != rns::api_version) {
    std::fprintf(stderr, "librns version mismatch: got %s want %s\n",
                 ver.c_str(), rns::api_version);
    return 1;
  }

  auto page_body = load_bytes(page_file, kFallbackPage);
  auto file_body = load_bytes(file_file, kFallbackFile);

  auto node_r = rns::Node::create(config_path);
  if (!node_r.ok()) {
    print_last_error("rns::Node::create failed");
    return 1;
  }
  auto node = std::move(node_r).value();

  auto id_r = load_or_create_identity(identity_path);
  if (!id_r.ok()) {
    print_last_error("identity load/create failed");
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

  auto dest_r =
      rns::Destination::create(node, nullptr, "nomadnetwork", {"node"}, true);
  if (!dest_r.ok()) {
    print_last_error("Destination::create failed");
    return 1;
  }
  auto dest = std::move(dest_r).value();

  if (dest.register_request_handler(request_path) != rns::Error::Ok) {
    print_last_error("register_request_handler failed");
    return 1;
  }
  if (dest.register_request_handler(file_path) != rns::Error::Ok) {
    print_last_error("register_request_handler file failed");
    return 1;
  }

  auto dest_hash = dest.hash();
  if (!dest_hash.ok()) {
    print_last_error("destination.hash failed");
    return 1;
  }
  auto dest_hex = rns::util::hash_to_hex(*dest_hash);
  if (!dest_hex.ok()) {
    std::fprintf(stderr, "failed to encode destination hash\n");
    return 1;
  }

  std::printf("DEST_HASH=%s\n", dest_hex->c_str());
  std::printf("REQUEST_PATH=%s\n", request_path.c_str());
  std::printf("FILE_PATH=%s\n", file_path.c_str());
  std::fprintf(stderr, "librns %s pageserver listening as nomadnetwork.node\n",
               ver.c_str());
  std::fprintf(stderr, "announce name=librns-cpp-pageserver interval=%ds\n",
               announce_sec);
  std::fprintf(stderr, "serving %zu bytes from %s\n", page_body.size(),
               page_file.c_str());
  std::fprintf(stderr, "serving %zu bytes from %s as %s\n", file_body.size(),
               file_file.c_str(), file_path.c_str());

  const char *app_data = "librns-cpp-pageserver";
  if (dest.announce(std::string_view(app_data)) != rns::Error::Ok) {
    print_last_error("destination.announce failed");
  } else {
    std::fprintf(stderr, "announce sent\n");
  }

  std::vector<std::uint8_t> req_buf(kReqDataCap);
  rns::span<std::uint8_t> req_span(req_buf.data(), req_buf.size());
  auto announce_every = std::chrono::seconds(announce_sec);
  auto last_announce = std::chrono::steady_clock::now();

  for (;;) {
    if (announce_sec > 0 &&
        std::chrono::steady_clock::now() - last_announce >= announce_every) {
      if (dest.announce(std::string_view(app_data)) == rns::Error::Ok) {
        std::fprintf(stderr, "announce refreshed\n");
      }
      last_announce = std::chrono::steady_clock::now();
    }

    auto ev = node.poll(200, req_span);
    if (!ev.ok()) {
      if (ev.error() == rns::Error::Timeout) {
        continue;
      }
      print_last_error("node.poll failed");
      return 1;
    }

    if (ev->kind() == rns::EventKind::LinkEstablished) {
      std::fprintf(stderr, "inbound link established\n");
    } else if (ev->kind() == rns::EventKind::LinkClosed) {
      std::fprintf(stderr, "link closed\n");
    } else if (ev->kind() == rns::EventKind::RequestIncoming) {
      auto path = ev->path();
      std::fprintf(stderr, "request incoming path=%.*s\n",
                   static_cast<int>(path.size()), path.data());
      auto req_id = ev->request_id();
      if (path == request_path) {
        if (rns::request_respond(node, req_id,
                                 rns::span<const std::uint8_t>(
                                     page_body.data(), page_body.size())) !=
            rns::Error::Ok) {
          print_last_error("request_respond failed");
        } else {
          std::fprintf(stderr, "served %s (%zu bytes)\n", request_path.c_str(),
                       page_body.size());
        }
      } else if (path == file_path) {
        if (rns::request_respond_file(
                node, req_id, "test.txt",
                rns::span<const std::uint8_t>(
                    file_body.data(), file_body.size())) != rns::Error::Ok) {
          print_last_error("request_respond_file failed");
        } else {
          std::fprintf(stderr, "served %s (%zu bytes)\n", file_path.c_str(),
                       file_body.size());
        }
      } else {
        const char *msg = "page not found\n";
        if (rns::request_respond(node, req_id, std::string_view(msg)) !=
            rns::Error::Ok) {
          print_last_error("request_respond failed");
        }
      }
    }
  }
}

} // namespace

int main(int argc, char **argv) {
  std::string config_path;
  std::string identity_path = kDefaultIdentityPath;
  std::string page_file = kDefaultPageFile;
  std::string file_file = kDefaultFileFile;
  std::string request_path = kDefaultPagePath;
  std::string file_path = kDefaultFilePath;
  int announce_sec = kDefaultAnnounceSec;

  for (int i = 1; i < argc; ++i) {
    std::string arg = argv[i];
    if (arg == "-c" && i + 1 < argc) {
      config_path = argv[++i];
      continue;
    }
    if (arg == "-i" && i + 1 < argc) {
      identity_path = argv[++i];
      continue;
    }
    if (arg == "-a" && i + 1 < argc) {
      announce_sec = std::atoi(argv[++i]);
      if (announce_sec < 0) {
        std::fprintf(stderr, "announce interval must be >= 0\n");
        return 1;
      }
      continue;
    }
    if (arg == "-p" && i + 1 < argc) {
      page_file = argv[++i];
      continue;
    }
    if (arg == "-f" && i + 1 < argc) {
      file_file = argv[++i];
      continue;
    }
    if (arg == "-P" && i + 1 < argc) {
      request_path = argv[++i];
      continue;
    }
    if (arg == "-h" || arg == "--help") {
      usage(argv[0]);
      return 0;
    }
    std::fprintf(stderr, "unknown option: %s\n", arg.c_str());
    usage(argv[0]);
    return 1;
  }

  if (config_path.empty()) {
    usage(argv[0]);
    return 1;
  }

  return run(config_path, identity_path, page_file, file_file, request_path,
             file_path, announce_sec);
}
