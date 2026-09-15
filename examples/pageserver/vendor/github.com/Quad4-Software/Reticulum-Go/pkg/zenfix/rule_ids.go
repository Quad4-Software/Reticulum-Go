// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

// Built-in rule identifiers. Checkers emit these IDs, metadata lives in AllRules.
const (
	RuleRequestPathLoop        = "zen/requestpath-loop"
	RuleRequestPathIgnoredErr  = "zen/requestpath-ignored-error"
	RuleHasPathLoop            = "zen/haspath-loop"
	RuleAwaitInLoop            = "zen/await-in-loop"
	RuleEstablishLoop          = "zen/establish-loop"
	RuleEstablishNoAwait       = "zen/establish-no-await"
	RuleEstablishRepeat        = "zen/establish-repeat"
	RuleNewLinkLoop            = "zen/newlink-loop"
	RuleNewLinkRepeat          = "zen/newlink-repeat"
	RuleLinkNotActive          = "zen/link-not-active"
	RuleLinkActiveUseLoop      = "zen/link-active-use-loop"
	RuleAnnounceLoop           = "zen/announce-loop"
	RuleFixed15sTimeout        = "zen/fixed-15s-timeout"
	RulePythonLinkSpin         = "zen/python-link-spin"
	RulePythonPathSpin         = "zen/python-path-spin"
	RulePythonRequestPathLoop  = "zen/python-requestpath-loop"
	RulePythonFixed15s         = "zen/python-fixed-15s"
	RuleOnInterfaceOverride    = "zen/on-interface-override"
	RuleRecallBeforePath       = "zen/recall-before-path"
	RulePythonAwaitInLoop      = "zen/python-await-in-loop"
	RulePythonLinkStatusSpin   = "zen/python-link-status-spin"
	RulePythonPathThenSpin     = "zen/python-path-request-then-spin"
	RulePythonRecallBeforePath = "zen/python-recall-before-path"
	RulePythonOnInterface      = "zen/python-on-interface-override"
	RulePythonRequireShared    = "zen/python-require-shared-instance"
)
