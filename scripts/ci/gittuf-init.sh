#!/bin/sh
# Initialize gittuf repository security policy.
#
# gittuf stores policy in git refs (refs/gittuf/*), not files, so this script
# must run inside the repository with the maintainer signing key available.
# After running, push the gittuf refs alongside the branches:
#
#   git push origin 'refs/gittuf/*'
#
# Note: the rns:// rngit remote may not mirror custom refs. Keep GitHub as
# the policy carrier or verify rngit propagates refs/gittuf/* before relying
# on it.
#
# Requires: gittuf on PATH (https://github.com/gittuf/gittuf) and a signing
# key. GPG keys, SSH keys, and sigstore identities all work; pass the key or
# identity via GITTUF_KEY (default: the repo user.signingkey).
#
# gittuf is pre-1.0 and its CLI surface changes between releases. Check
# gittuf --help if a flag below has been renamed.
set -eu

if ! command -v gittuf >/dev/null 2>&1; then
	echo "gittuf-init: gittuf not on PATH" >&2
	echo "gittuf-init: install a pinned release from https://github.com/gittuf/gittuf/releases" >&2
	exit 1
fi

KEY="${GITTUF_KEY:-$(git config user.signingkey || true)}"
if [ -z "$KEY" ]; then
	echo "gittuf-init: no signing key; set GITTUF_KEY or git config user.signingkey" >&2
	exit 1
fi

echo "gittuf-init: using key $KEY"

# Root of trust: the maintainer key is both root and policy signer.
gittuf trust init --signing-key "$KEY"
gittuf policy init --signing-key "$KEY"

# Branch protection: only the maintainer key may update master and dev.
for ref in master dev; do
	gittuf policy add-rule --signing-key "$KEY" \
		--rule-name "protect-$ref" \
		--rule-pattern "git:refs/heads/$ref" \
		--authorize-key "$KEY"
done

# Tag protection: only signed tags from the maintainer key count as releases.
gittuf policy add-rule --signing-key "$KEY" \
	--rule-name "protect-tags" \
	--rule-pattern "git:refs/tags/*" \
	--authorize-key "$KEY"

# File rules: crypto, identity and CI plumbing need the maintainer key even
# on unprotected branches. Adjust patterns as the tree changes.
for path in "file:pkg/cryptography/*" "file:pkg/identity/*" "file:.github/workflows/*" "file:scripts/ci/*"; do
	gittuf policy add-rule --signing-key "$KEY" \
		--rule-name "protect-${path#file:}" \
		--rule-pattern "$path" \
		--authorize-key "$KEY"
done

# Record the policy in the reference state log.
gittuf policy apply --signing-key "$KEY"

echo "gittuf-init: done. Push refs/gittuf/* and enable verification in CI."
