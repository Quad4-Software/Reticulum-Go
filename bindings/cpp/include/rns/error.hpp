// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_ERROR_HPP
#define RNS_ERROR_HPP

#include <cstddef>
#include <string>
#include <utility>

#include "rns/detail/c_api.hpp"

namespace rns {

enum class Error {
	Ok = RNS_OK,
	InvalidArg = RNS_ERR_INVALID_ARG,
	InvalidHandle = RNS_ERR_INVALID_HANDLE,
	NotFound = RNS_ERR_NOT_FOUND,
	State = RNS_ERR_STATE,
	Io = RNS_ERR_IO,
	Internal = RNS_ERR_INTERNAL,
	Timeout = RNS_ERR_TIMEOUT,
	Truncated = RNS_ERR_TRUNCATED,
};

inline Error map_code(int code) noexcept {
	switch (code) {
	case RNS_OK:
		return Error::Ok;
	case RNS_ERR_INVALID_ARG:
		return Error::InvalidArg;
	case RNS_ERR_INVALID_HANDLE:
		return Error::InvalidHandle;
	case RNS_ERR_NOT_FOUND:
		return Error::NotFound;
	case RNS_ERR_STATE:
		return Error::State;
	case RNS_ERR_IO:
		return Error::Io;
	case RNS_ERR_INTERNAL:
		return Error::Internal;
	case RNS_ERR_TIMEOUT:
		return Error::Timeout;
	case RNS_ERR_TRUNCATED:
		return Error::Truncated;
	default:
		return Error::Internal;
	}
}

inline const char *error_string(Error err) noexcept {
	switch (err) {
	case Error::Ok:
		return "ok";
	case Error::InvalidArg:
		return "invalid argument";
	case Error::InvalidHandle:
		return "invalid handle";
	case Error::NotFound:
		return "not found";
	case Error::State:
		return "invalid state";
	case Error::Io:
		return "io error";
	case Error::Internal:
		return "internal error";
	case Error::Timeout:
		return "timeout";
	case Error::Truncated:
		return "truncated";
	}
	return "internal error";
}

inline std::string version() {
	const char *v = rns_version();
	return v ? std::string(v) : std::string();
}

inline std::string last_error() {
	char buf[512];
	std::size_t written = 0;
	if (rns_last_error(buf, sizeof(buf), &written) != RNS_OK) {
		return {};
	}
	if (written > sizeof(buf)) {
		written = sizeof(buf);
	}
	return std::string(buf, written);
}

template <typename T> class Result {
      public:
	Result(T value) : ok_(true), value_(std::move(value)), err_(Error::Ok) {}

	Result(Error err) : ok_(false), value_(), err_(err) {}

	bool ok() const noexcept { return ok_; }
	explicit operator bool() const noexcept { return ok_; }

	Error error() const noexcept { return err_; }

	T &value() & { return value_; }
	const T &value() const & { return value_; }
	T &&value() && { return std::move(value_); }

	T *operator->() { return &value_; }
	const T *operator->() const { return &value_; }

	T &operator*() & { return value_; }
	const T &operator*() const & { return value_; }

      private:
	bool ok_;
	T value_;
	Error err_;
};

} // namespace rns

#endif
