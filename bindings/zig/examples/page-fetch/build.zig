// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const rns_dep = b.dependency("rns", .{
        .target = target,
        .optimize = optimize,
    });
    const rns_mod = rns_dep.module("rns");

    const librns_dir = b.pathFromRoot("../../../../bin");

    const exe = b.addExecutable(.{
        .name = "zig-page-fetch",
        .root_module = b.createModule(.{
            .root_source_file = b.path("main.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
            .imports = &.{
                .{ .name = "rns", .module = rns_mod },
            },
        }),
    });
    exe.root_module.addLibraryPath(.{ .cwd_relative = librns_dir });
    exe.root_module.addRPath(.{ .cwd_relative = librns_dir });
    exe.root_module.linkSystemLibrary("rns", .{});

    b.installArtifact(exe);

    const run_cmd = b.addRunArtifact(exe);
    run_cmd.setEnvironmentVariable("LD_LIBRARY_PATH", librns_dir);
    if (b.args) |args| run_cmd.addArgs(args);
    const run_step = b.step("run", "Run zig-page-fetch");
    run_step.dependOn(&run_cmd.step);
}
