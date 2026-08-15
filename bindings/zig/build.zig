// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const librns_dir = b.pathFromRoot("../../bin");

    const rns_mod = b.addModule("rns", .{
        .root_source_file = b.path("src/root.zig"),
        .target = target,
        .optimize = optimize,
        .link_libc = true,
    });
    rns_mod.addLibraryPath(.{ .cwd_relative = librns_dir });
    rns_mod.addRPath(.{ .cwd_relative = librns_dir });
    rns_mod.linkSystemLibrary("rns", .{});

    const test_mod = b.createModule(.{
        .root_source_file = b.path("tests/root.zig"),
        .target = target,
        .optimize = optimize,
        .link_libc = true,
        .imports = &.{
            .{ .name = "rns", .module = rns_mod },
        },
    });
    test_mod.addLibraryPath(.{ .cwd_relative = librns_dir });
    test_mod.addRPath(.{ .cwd_relative = librns_dir });
    test_mod.linkSystemLibrary("rns", .{});

    const tests = b.addTest(.{
        .root_module = test_mod,
    });

    const run_tests = b.addRunArtifact(tests);
    run_tests.setEnvironmentVariable("LD_LIBRARY_PATH", librns_dir);

    const test_step = b.step("test", "Run Zig librns binding tests");
    test_step.dependOn(&run_tests.step);
}
