// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#include "rns/event.hpp"

extern "C" void rns_cpp_event_trampoline(const rns_event *event, void *user_data) {
	if (event == nullptr || user_data == nullptr) {
		return;
	}
	auto *cb = static_cast<rns::EventCallback *>(user_data);
	if (!(*cb)) {
		return;
	}
	(*cb)(rns::Event::from_c(*event));
}
