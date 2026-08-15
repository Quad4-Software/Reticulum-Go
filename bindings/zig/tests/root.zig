// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

comptime {
    _ = @import("smoke_test.zig");
    _ = @import("identity_test.zig");
    _ = @import("destination_test.zig");
    _ = @import("link_udp_test.zig");
}
