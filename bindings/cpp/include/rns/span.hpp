// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#ifndef RNS_SPAN_HPP
#define RNS_SPAN_HPP

#include <cstddef>
#include <type_traits>

namespace rns {

// Minimal C++17 span (pointer + size). Not a full std::span replacement.
template <typename T> class span {
      public:
	using element_type = T;
	using value_type = typename std::remove_cv<T>::type;
	using size_type = std::size_t;
	using pointer = T *;
	using reference = T &;
	using iterator = T *;
	using const_iterator = const T *;

	span() noexcept : data_(nullptr), size_(0) {}

	span(T *data, size_type size) noexcept : data_(data), size_(size) {}

	template <std::size_t N> span(T (&arr)[N]) noexcept : data_(arr), size_(N) {}

	template <typename U, std::size_t N>
	span(U (&arr)[N],
	     typename std::enable_if<std::is_convertible<U (*)[], T (*)[]>::value>::type * =
		 nullptr) noexcept
	    : data_(arr), size_(N) {}

	T *data() const noexcept { return data_; }
	size_type size() const noexcept { return size_; }
	bool empty() const noexcept { return size_ == 0; }

	reference operator[](size_type i) const { return data_[i]; }

	iterator begin() const noexcept { return data_; }
	iterator end() const noexcept { return data_ + size_; }
	const_iterator cbegin() const noexcept { return data_; }
	const_iterator cend() const noexcept { return data_ + size_; }

      private:
	T *data_;
	size_type size_;
};

} // namespace rns

#endif
