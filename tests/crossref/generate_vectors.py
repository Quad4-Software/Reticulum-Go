#!/usr/bin/env python3
"""
Generates test vectors from the Python Reticulum reference implementation.
These vectors are consumed by the Go implementation's cross-reference tests
to ensure protocol compatibility between implementations.

Usage:
    python3 generate_vectors.py

Environment:
    RETICULUM_PATH  Path to Python Reticulum repo (default: reticulum-ref/ in project root)

Output:
    test_vectors.json in the same directory

To run full crossref test (clones/updates from github.com/markqvist/Reticulum):
    make test-crossref
    task test-crossref
    ./tests/crossref/run_crossref.sh
"""

import sys
import os
import json
import hashlib
import struct

_reticulum_path = os.environ.get(
    "RETICULUM_PATH",
    os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "reticulum-ref")),
)
sys.path.insert(0, _reticulum_path)

from RNS.Cryptography import X25519PrivateKey, X25519PublicKey, Ed25519PrivateKey, Ed25519PublicKey
from RNS.Cryptography import hkdf, Token
from RNS.Cryptography.HMAC import HMAC
from RNS.Identity import Identity


def generate_identity_vectors():
    """Generate identity-related test vectors from known seeds."""
    vectors = []

    for i in range(3):
        # Use deterministic seed material
        seed = hashlib.sha512(f"reticulum-crossref-test-seed-{i}".encode()).digest()
        x25519_seed = seed[:32]
        ed25519_seed = seed[32:64]
        prv_bytes = x25519_seed + ed25519_seed

        identity = Identity(create_keys=False)
        identity.load_private_key(prv_bytes)

        pub_key = identity.get_public_key()
        prv_key = identity.get_private_key()
        id_hash = identity.hash
        id_hexhash = identity.hexhash

        # Sign a known message
        test_message = f"test message {i}".encode()
        signature = identity.sign(test_message)

        vectors.append({
            "private_key_hex": prv_key.hex(),
            "public_key_hex": pub_key.hex(),
            "hash_hex": id_hash.hex(),
            "hexhash": id_hexhash,
            "sign_message_hex": test_message.hex(),
            "signature_hex": signature.hex(),
        })

    return vectors


def generate_destination_hash_vectors():
    """Generate destination hash test vectors."""
    vectors = []

    seed = hashlib.sha512(b"reticulum-crossref-test-seed-0").digest()
    prv_bytes = seed[:64]
    identity = Identity(create_keys=False)
    identity.load_private_key(prv_bytes)

    test_cases = [
        ("testapp", []),
        ("testapp", ["echo"]),
        ("testapp", ["echo", "request"]),
        ("myapp", ["service", "v1"]),
        ("lxmf", ["delivery"]),
    ]

    for app_name, aspects in test_cases:
        # Compute name without identity (for expand_name hash)
        from RNS.Destination import Destination
        full_name = Destination.expand_name(None, app_name, *aspects)
        name_hash_full = hashlib.sha256(full_name.encode("utf-8")).digest()
        name_hash_10 = name_hash_full[:10]

        # SINGLE destination hash (with identity)
        addr_hash_material = name_hash_10 + identity.hash
        dest_hash = hashlib.sha256(addr_hash_material).digest()[:16]

        # PLAIN destination hash (no identity)
        plain_hash = hashlib.sha256(name_hash_10).digest()[:16]

        vectors.append({
            "app_name": app_name,
            "aspects": aspects,
            "expand_name": full_name,
            "name_hash_10_hex": name_hash_10.hex(),
            "identity_hash_hex": identity.hash.hex(),
            "single_dest_hash_hex": dest_hash.hex(),
            "plain_dest_hash_hex": plain_hash.hex(),
        })

    return vectors


def generate_hkdf_vectors():
    """Generate HKDF test vectors."""
    vectors = []

    test_cases = [
        (b"\x0b" * 32, b"\x00" * 16, b"", 32),
        (b"\x0b" * 32, b"\x00" * 16, b"context-info", 64),
        (b"shared-secret-material-here!!!!!", b"salt-value-here!", None, 64),
        (b"\x01" * 32, None, None, 48),
    ]

    for secret, salt, context, length in test_cases:
        derived = hkdf(length=length, derive_from=secret, salt=salt, context=context)
        vectors.append({
            "secret_hex": secret.hex(),
            "salt_hex": salt.hex() if salt else "",
            "context_hex": context.hex() if context else "",
            "length": length,
            "derived_hex": derived.hex(),
        })

    return vectors


def generate_hmac_vectors():
    """Generate HMAC-SHA256 test vectors."""
    vectors = []

    test_cases = [
        (b"key-material-sixteen", b"hello world"),
        (b"\x00" * 32, b"\xff" * 64),
        (b"reticulum-hmac-key!!", b"authenticate this data please"),
    ]

    for key, message in test_cases:
        mac = HMAC(key, message).digest()
        vectors.append({
            "key_hex": key.hex(),
            "message_hex": message.hex(),
            "hmac_hex": mac.hex(),
        })

    return vectors


def generate_token_vectors():
    """Generate Token (Fernet-like) encrypt/decrypt test vectors with known IV."""
    vectors = []

    # We can't control the IV in Token.encrypt(), so instead we test
    # the structural format: given a 64-byte key, encrypt plaintext
    # and verify the output structure is [iv:16][ciphertext][hmac:32]
    key = bytes(range(64))  # deterministic key
    plaintext = b"hello reticulum protocol"

    token = Token(key)
    encrypted = token.encrypt(plaintext)

    # Verify we can decrypt
    decrypted = token.decrypt(encrypted)
    assert decrypted == plaintext

    vectors.append({
        "key_hex": key.hex(),
        "plaintext_hex": plaintext.hex(),
        "token_hex": encrypted.hex(),
        "iv_hex": encrypted[:16].hex(),
        "ciphertext_hex": encrypted[16:-32].hex(),
        "hmac_hex": encrypted[-32:].hex(),
        "token_overhead": 48,
    })

    return vectors


def generate_packet_header_vectors():
    """Generate packet header flag encoding vectors."""
    vectors = []

    test_cases = [
        # (header_type, context_flag, transport_type, dest_type, packet_type)
        (0x00, 0x00, 0x00, 0x00, 0x00),  # HEADER_1, no flag, BROADCAST, SINGLE, DATA
        (0x00, 0x00, 0x00, 0x00, 0x01),  # HEADER_1, no flag, BROADCAST, SINGLE, ANNOUNCE
        (0x01, 0x00, 0x01, 0x00, 0x00),  # HEADER_2, no flag, TRANSPORT, SINGLE, DATA
        (0x00, 0x01, 0x00, 0x02, 0x00),  # HEADER_1, FLAG_SET, BROADCAST, PLAIN, DATA
        (0x00, 0x00, 0x00, 0x03, 0x02),  # HEADER_1, no flag, BROADCAST, LINK, LINKREQUEST
        (0x00, 0x01, 0x00, 0x00, 0x01),  # HEADER_1, FLAG_SET, BROADCAST, SINGLE, ANNOUNCE (ratchet)
    ]

    for ht, cf, tt, dt, pt in test_cases:
        flags = (ht << 6) | (cf << 5) | (tt << 4) | (dt << 2) | pt
        vectors.append({
            "header_type": ht,
            "context_flag": cf,
            "transport_type": tt,
            "destination_type": dt,
            "packet_type": pt,
            "flags_byte": flags,
        })

    return vectors


def generate_announce_vectors():
    """Generate announce structure vectors."""
    vectors = []

    seed = hashlib.sha512(b"reticulum-crossref-test-seed-0").digest()
    prv_bytes = seed[:64]
    identity = Identity(create_keys=False)
    identity.load_private_key(prv_bytes)

    pub_key = identity.get_public_key()

    app_name = "testapp"
    aspects = ["echo"]
    app_data = b"announce app data"

    from RNS.Destination import Destination
    full_name = Destination.expand_name(None, app_name, *aspects)
    name_hash = hashlib.sha256(full_name.encode("utf-8")).digest()[:10]

    addr_material = name_hash + identity.hash
    dest_hash = hashlib.sha256(addr_material).digest()[:16]

    # Random hash (deterministic for testing)
    random_hash = hashlib.sha256(b"deterministic-random-hash-seed").digest()[:10]

    # No ratchet case
    signed_data = dest_hash + pub_key + name_hash + random_hash + app_data
    signature = identity.sign(signed_data)

    payload_no_ratchet = pub_key + name_hash + random_hash + signature + app_data

    vectors.append({
        "has_ratchet": False,
        "public_key_hex": pub_key.hex(),
        "name_hash_hex": name_hash.hex(),
        "random_hash_hex": random_hash.hex(),
        "dest_hash_hex": dest_hash.hex(),
        "app_data_hex": app_data.hex(),
        "signed_data_hex": signed_data.hex(),
        "signature_hex": signature.hex(),
        "payload_hex": payload_no_ratchet.hex(),
    })

    # With ratchet case
    ratchet_key = hashlib.sha256(b"deterministic-ratchet-seed").digest()
    signed_data_r = dest_hash + pub_key + name_hash + random_hash + ratchet_key + app_data
    signature_r = identity.sign(signed_data_r)

    payload_ratchet = pub_key + name_hash + random_hash + ratchet_key + signature_r + app_data

    vectors.append({
        "has_ratchet": True,
        "public_key_hex": pub_key.hex(),
        "name_hash_hex": name_hash.hex(),
        "random_hash_hex": random_hash.hex(),
        "ratchet_hex": ratchet_key.hex(),
        "dest_hash_hex": dest_hash.hex(),
        "app_data_hex": app_data.hex(),
        "signed_data_hex": signed_data_r.hex(),
        "signature_hex": signature_r.hex(),
        "payload_hex": payload_ratchet.hex(),
    })

    return vectors


def generate_encryption_vectors():
    """Generate cross-implementation encryption test vectors.
    Python encrypts -> Go should decrypt, using the identity encryption scheme.
    """
    vectors = []

    seed = hashlib.sha512(b"reticulum-crossref-test-seed-0").digest()
    prv_bytes = seed[:64]
    identity = Identity(create_keys=False)
    identity.load_private_key(prv_bytes)

    test_messages = [
        b"short",
        b"hello reticulum cross-implementation test",
        b"\x00" * 100,
        bytes(range(256)) * 2,
    ]

    for msg in test_messages:
        ciphertext = identity.encrypt(msg)
        decrypted = identity.decrypt(ciphertext)
        assert decrypted == msg, f"Self-decrypt failed for message of length {len(msg)}"

        vectors.append({
            "private_key_hex": identity.get_private_key().hex(),
            "public_key_hex": identity.get_public_key().hex(),
            "plaintext_hex": msg.hex(),
            "ciphertext_hex": ciphertext.hex(),
        })

    return vectors


def generate_hash_vectors():
    """Generate SHA-256 full and truncated hash vectors."""
    vectors = []

    test_data = [
        b"",
        b"hello",
        b"reticulum",
        bytes(range(256)),
    ]

    for data in test_data:
        full = hashlib.sha256(data).digest()
        truncated = full[:16]
        vectors.append({
            "input_hex": data.hex(),
            "full_hash_hex": full.hex(),
            "truncated_hash_hex": truncated.hex(),
        })

    return vectors


def generate_ecdh_vectors():
    """Generate ECDH shared secret vectors between two keypairs."""
    vectors = []

    seeds = [
        hashlib.sha512(b"ecdh-test-keypair-A").digest()[:32],
        hashlib.sha512(b"ecdh-test-keypair-B").digest()[:32],
        hashlib.sha512(b"ecdh-test-keypair-C").digest()[:32],
    ]

    pairs = [(0, 1), (0, 2), (1, 2)]
    keys = []
    for s in seeds:
        prv = X25519PrivateKey.from_private_bytes(s)
        pub = prv.public_key()
        keys.append((s, prv, pub))

    for a, b in pairs:
        shared_ab = keys[a][1].exchange(keys[b][2])
        shared_ba = keys[b][1].exchange(keys[a][2])
        assert shared_ab == shared_ba, "ECDH shared secret mismatch"

        vectors.append({
            "private_a_hex": keys[a][0].hex(),
            "public_a_hex": keys[a][2].public_bytes().hex(),
            "private_b_hex": keys[b][0].hex(),
            "public_b_hex": keys[b][2].public_bytes().hex(),
            "shared_secret_hex": shared_ab.hex(),
        })

    return vectors


def generate_ratchet_id_vectors():
    """Generate ratchet ID computation vectors."""
    vectors = []

    test_seeds = [
        hashlib.sha256(b"ratchet-seed-0").digest(),
        hashlib.sha256(b"ratchet-seed-1").digest(),
        hashlib.sha256(b"ratchet-seed-2").digest(),
    ]

    for seed in test_seeds:
        ratchet_prv = X25519PrivateKey.from_private_bytes(seed)
        ratchet_pub_bytes = ratchet_prv.public_key().public_bytes()
        ratchet_id = Identity.full_hash(ratchet_pub_bytes)[:Identity.NAME_HASH_LENGTH // 8]

        vectors.append({
            "ratchet_private_hex": seed.hex(),
            "ratchet_public_hex": ratchet_pub_bytes.hex(),
            "ratchet_id_hex": ratchet_id.hex(),
        })

    return vectors


def generate_packet_wire_vectors():
    """Generate raw packet wire format vectors for parsing."""
    vectors = []

    dest_hash = hashlib.sha256(b"test-destination").digest()[:16]
    transport_id = hashlib.sha256(b"test-transport").digest()[:16]

    # HEADER_1, DATA, SINGLE, BROADCAST
    flags = (0x00 << 6) | (0x00 << 5) | (0x00 << 4) | (0x00 << 2) | 0x00
    hops = 3
    context = 0x00
    data = b"hello from python"
    raw = struct.pack("!BB", flags, hops) + dest_hash + struct.pack("!B", context) + data

    vectors.append({
        "raw_hex": raw.hex(),
        "header_type": 0,
        "packet_type": 0,
        "transport_type": 0,
        "destination_type": 0,
        "context_flag": 0,
        "context": context,
        "hops": hops,
        "dest_hash_hex": dest_hash.hex(),
        "transport_id_hex": "",
        "data_hex": data.hex(),
    })

    # HEADER_1, ANNOUNCE, SINGLE, BROADCAST (with context flag set for ratchet)
    flags2 = (0x00 << 6) | (0x01 << 5) | (0x00 << 4) | (0x00 << 2) | 0x01
    context2 = 0x00
    data2 = b"announce payload"
    raw2 = struct.pack("!BB", flags2, 0) + dest_hash + struct.pack("!B", context2) + data2

    vectors.append({
        "raw_hex": raw2.hex(),
        "header_type": 0,
        "packet_type": 1,
        "transport_type": 0,
        "destination_type": 0,
        "context_flag": 1,
        "context": context2,
        "hops": 0,
        "dest_hash_hex": dest_hash.hex(),
        "transport_id_hex": "",
        "data_hex": data2.hex(),
    })

    # HEADER_2, DATA, SINGLE, TRANSPORT
    flags3 = (0x01 << 6) | (0x00 << 5) | (0x01 << 4) | (0x00 << 2) | 0x00
    hops3 = 7
    context3 = 0x0B  # PATH_RESPONSE
    data3 = b"transport data"
    raw3 = struct.pack("!BB", flags3, hops3) + transport_id + dest_hash + struct.pack("!B", context3) + data3

    vectors.append({
        "raw_hex": raw3.hex(),
        "header_type": 1,
        "packet_type": 0,
        "transport_type": 1,
        "destination_type": 0,
        "context_flag": 0,
        "context": context3,
        "hops": hops3,
        "dest_hash_hex": dest_hash.hex(),
        "transport_id_hex": transport_id.hex(),
        "data_hex": data3.hex(),
    })

    # HEADER_1, LINKREQUEST, LINK, BROADCAST
    flags4 = (0x00 << 6) | (0x00 << 5) | (0x00 << 4) | (0x03 << 2) | 0x02
    data4 = b"link request payload"
    raw4 = struct.pack("!BB", flags4, 0) + dest_hash + struct.pack("!B", 0x00) + data4

    vectors.append({
        "raw_hex": raw4.hex(),
        "header_type": 0,
        "packet_type": 2,
        "transport_type": 0,
        "destination_type": 3,
        "context_flag": 0,
        "context": 0x00,
        "hops": 0,
        "dest_hash_hex": dest_hash.hex(),
        "transport_id_hex": "",
        "data_hex": data4.hex(),
    })

    # HEADER_1, PROOF, SINGLE, BROADCAST
    flags5 = (0x00 << 6) | (0x00 << 5) | (0x00 << 4) | (0x00 << 2) | 0x03
    data5 = b"proof data"
    raw5 = struct.pack("!BB", flags5, 2) + dest_hash + struct.pack("!B", 0x00) + data5

    vectors.append({
        "raw_hex": raw5.hex(),
        "header_type": 0,
        "packet_type": 3,
        "transport_type": 0,
        "destination_type": 0,
        "context_flag": 0,
        "context": 0x00,
        "hops": 2,
        "dest_hash_hex": dest_hash.hex(),
        "transport_id_hex": "",
        "data_hex": data5.hex(),
    })

    return vectors


def generate_packet_hash_vectors():
    """Generate packet hash vectors from raw packet bytes."""
    vectors = []

    dest_hash = hashlib.sha256(b"test-destination").digest()[:16]

    # HEADER_1 packet
    flags = (0x00 << 6) | (0x00 << 5) | (0x00 << 4) | (0x00 << 2) | 0x00
    hops = 0
    context = 0x00
    data = b"packet for hashing"
    raw = struct.pack("!BB", flags, hops) + dest_hash + struct.pack("!B", context) + data

    hashable_flags = flags & 0x0F
    hashable = struct.pack("!B", hashable_flags) + raw[2:]
    pkt_hash = hashlib.sha256(hashable).digest()
    truncated = pkt_hash[:16]

    vectors.append({
        "raw_hex": raw.hex(),
        "header_type": 0,
        "packet_hash_hex": pkt_hash.hex(),
        "truncated_hash_hex": truncated.hex(),
    })

    # HEADER_2 packet (transport)
    transport_id = hashlib.sha256(b"test-transport").digest()[:16]
    flags2 = (0x01 << 6) | (0x00 << 5) | (0x01 << 4) | (0x00 << 2) | 0x00
    hops2 = 5
    context2 = 0x00
    data2 = b"transported data"
    raw2 = struct.pack("!BB", flags2, hops2) + transport_id + dest_hash + struct.pack("!B", context2) + data2

    hashable_flags2 = flags2 & 0x0F
    start_idx = 16 + 2  # skip flags, hops, transport_id
    hashable2 = struct.pack("!B", hashable_flags2) + raw2[start_idx:]
    pkt_hash2 = hashlib.sha256(hashable2).digest()
    truncated2 = pkt_hash2[:16]

    vectors.append({
        "raw_hex": raw2.hex(),
        "header_type": 1,
        "packet_hash_hex": pkt_hash2.hex(),
        "truncated_hash_hex": truncated2.hex(),
    })

    return vectors


def generate_aes_vectors():
    """Generate AES-256-CBC test vectors with known key and IV."""
    from RNS.Cryptography.AES import AES_256_CBC
    from RNS.Cryptography.PKCS7 import PKCS7

    vectors = []

    test_cases = [
        (bytes(range(32)), bytes(range(16)), b"sixteen bytes!!"),
        (bytes(range(32)), bytes(range(16, 32)), b"exactly sixteen!"),
        (bytes(range(32)), b"\xaa" * 16, b"a"),
        (bytes(range(32)), b"\xbb" * 16, b"longer plaintext that spans multiple blocks for testing"),
    ]

    for key, iv, plaintext in test_cases:
        padded = PKCS7.pad(plaintext)
        ciphertext = AES_256_CBC.encrypt(plaintext=padded, key=key, iv=iv)

        decrypted = PKCS7.unpad(AES_256_CBC.decrypt(ciphertext=ciphertext, key=key, iv=iv))
        assert decrypted == plaintext, f"AES self-test failed"

        vectors.append({
            "key_hex": key.hex(),
            "iv_hex": iv.hex(),
            "plaintext_hex": plaintext.hex(),
            "padded_hex": padded.hex(),
            "ciphertext_hex": ciphertext.hex(),
        })

    return vectors


def generate_cross_sign_vectors():
    """Generate vectors where multiple identities sign each other's data."""
    vectors = []

    identities = []
    for i in range(3):
        seed = hashlib.sha512(f"reticulum-crossref-test-seed-{i}".encode()).digest()
        prv_bytes = seed[:64]
        ident = Identity(create_keys=False)
        ident.load_private_key(prv_bytes)
        identities.append(ident)

    test_data = [
        b"",
        b"\x00",
        b"A" * 500,
        bytes(range(256)),
        b"cross-implementation signature verification test",
    ]

    for signer_idx, signer in enumerate(identities):
        for data in test_data:
            sig = signer.sign(data)
            for verifier_idx, verifier in enumerate(identities):
                valid = verifier.validate(sig, data) if signer_idx == verifier_idx else not verifier.validate(sig, data)
                # Only store signer's own vectors for Go to verify
            vectors.append({
                "signer_index": signer_idx,
                "signer_public_key_hex": signer.get_public_key().hex(),
                "data_hex": data.hex(),
                "signature_hex": sig.hex(),
            })

    return vectors


def generate_identity_file_vectors():
    """Generate identity file format vectors (64 bytes: private key material)."""
    vectors = []

    for i in range(3):
        seed = hashlib.sha512(f"reticulum-crossref-test-seed-{i}".encode()).digest()
        prv_bytes = seed[:64]
        ident = Identity(create_keys=False)
        ident.load_private_key(prv_bytes)

        file_bytes = ident.get_private_key()
        assert len(file_bytes) == 64

        vectors.append({
            "file_bytes_hex": file_bytes.hex(),
            "public_key_hex": ident.get_public_key().hex(),
            "hash_hex": ident.hash.hex(),
        })

    return vectors


def generate_link_signalling_vectors():
    """Generate link MTU signalling byte vectors."""
    vectors = []

    test_cases = [
        (500, 0x01),    # default MTU, AES-256-CBC
        (500, 0x00),    # default MTU, AES-128-CBC
        (1064, 0x01),   # larger MTU, AES-256-CBC
        (0, 0x01),      # zero MTU
        (2097151, 0x01), # max 21-bit MTU (0x1FFFFF)
    ]

    for mtu, mode in test_cases:
        signalling_value = (mtu & 0x1FFFFF) + (((mode << 5) & 0xE0) << 16)
        signalling_bytes = struct.pack(">I", signalling_value)[1:]

        decoded_mtu = ((signalling_bytes[0] << 16) + (signalling_bytes[1] << 8) + signalling_bytes[2]) & 0x1FFFFF
        decoded_mode = (signalling_bytes[0] & 0xE0) >> 5

        vectors.append({
            "mtu": mtu,
            "mode": mode,
            "signalling_hex": signalling_bytes.hex(),
            "decoded_mtu": decoded_mtu,
            "decoded_mode": decoded_mode,
        })

    return vectors


def generate_link_key_derivation_vectors():
    """Generate link handshake key derivation vectors."""
    vectors = []

    # Simulate link key derivation: ECDH shared secret + HKDF with link_id as salt
    seeds = [
        hashlib.sha512(b"link-initiator-key-seed").digest()[:32],
        hashlib.sha512(b"link-responder-key-seed").digest()[:32],
    ]

    initiator_prv = X25519PrivateKey.from_private_bytes(seeds[0])
    responder_prv = X25519PrivateKey.from_private_bytes(seeds[1])

    initiator_pub = initiator_prv.public_key().public_bytes()
    responder_pub = responder_prv.public_key().public_bytes()

    shared_key = initiator_prv.exchange(responder_prv.public_key())

    link_id = hashlib.sha256(b"test-link-id-material").digest()[:16]

    # MODE_AES256_CBC: 64-byte derived key
    derived_64 = hkdf(length=64, derive_from=shared_key, salt=link_id, context=None)
    hmac_key_256 = derived_64[:32]
    session_key_256 = derived_64[32:64]

    vectors.append({
        "initiator_prv_hex": seeds[0].hex(),
        "initiator_pub_hex": initiator_pub.hex(),
        "responder_prv_hex": seeds[1].hex(),
        "responder_pub_hex": responder_pub.hex(),
        "shared_key_hex": shared_key.hex(),
        "link_id_hex": link_id.hex(),
        "mode": 0x01,
        "derived_key_hex": derived_64.hex(),
        "hmac_key_hex": hmac_key_256.hex(),
        "session_key_hex": session_key_256.hex(),
    })

    # MODE_AES128_CBC: 32-byte derived key
    derived_32 = hkdf(length=32, derive_from=shared_key, salt=link_id, context=None)
    hmac_key_128 = derived_32[:16]
    session_key_128 = derived_32[16:32]

    vectors.append({
        "initiator_prv_hex": seeds[0].hex(),
        "initiator_pub_hex": initiator_pub.hex(),
        "responder_prv_hex": seeds[1].hex(),
        "responder_pub_hex": responder_pub.hex(),
        "shared_key_hex": shared_key.hex(),
        "link_id_hex": link_id.hex(),
        "mode": 0x00,
        "derived_key_hex": derived_32.hex(),
        "hmac_key_hex": hmac_key_128.hex(),
        "session_key_hex": session_key_128.hex(),
    })

    return vectors


def generate_link_request_vectors():
    """Generate link request payload format vectors."""
    vectors = []

    for i in range(2):
        seed = hashlib.sha512(f"link-request-seed-{i}".encode()).digest()
        x25519_seed = seed[:32]
        ed25519_seed = seed[32:64]

        x25519_prv = X25519PrivateKey.from_private_bytes(x25519_seed)
        x25519_pub = x25519_prv.public_key().public_bytes()

        ed25519_prv = Ed25519PrivateKey.from_private_bytes(ed25519_seed)
        ed25519_pub = ed25519_prv.public_key().public_bytes()

        mtu = 500
        mode = 0x01  # AES-256-CBC
        signalling_value = (mtu & 0x1FFFFF) + (((mode << 5) & 0xE0) << 16)
        signalling = struct.pack(">I", signalling_value)[1:]

        # Link request payload: [X25519_pub(32) | Ed25519_pub(32) | signalling(3)]
        payload = x25519_pub + ed25519_pub + signalling

        vectors.append({
            "x25519_pub_hex": x25519_pub.hex(),
            "ed25519_pub_hex": ed25519_pub.hex(),
            "signalling_hex": signalling.hex(),
            "payload_hex": payload.hex(),
            "ecpubsize": 64,
            "payload_len": len(payload),
        })

    return vectors


def generate_link_proof_vectors():
    """Generate link proof format vectors."""
    vectors = []

    seed = hashlib.sha512(b"reticulum-crossref-test-seed-0").digest()
    prv_bytes = seed[:64]
    signer = Identity(create_keys=False)
    signer.load_private_key(prv_bytes)

    x25519_seed = hashlib.sha512(b"link-proof-x25519-seed").digest()[:32]
    x25519_prv = X25519PrivateKey.from_private_bytes(x25519_seed)
    x25519_pub = x25519_prv.public_key().public_bytes()

    ed25519_pub = signer.sig_pub_bytes

    link_id = hashlib.sha256(b"link-proof-link-id").digest()[:16]

    mtu = 500
    mode = 0x01
    signalling_value = (mtu & 0x1FFFFF) + (((mode << 5) & 0xE0) << 16)
    signalling = struct.pack(">I", signalling_value)[1:]

    # Signed data: [link_id | pub | sig_pub | signalling]
    signed_data = link_id + x25519_pub + ed25519_pub + signalling
    signature = signer.sign(signed_data)

    # Proof payload: [signature(64) | pub(32) | signalling(3)]
    proof_payload = signature + x25519_pub + signalling

    vectors.append({
        "signer_public_key_hex": signer.get_public_key().hex(),
        "link_id_hex": link_id.hex(),
        "x25519_pub_hex": x25519_pub.hex(),
        "ed25519_pub_hex": ed25519_pub.hex(),
        "signalling_hex": signalling.hex(),
        "signed_data_hex": signed_data.hex(),
        "signature_hex": signature.hex(),
        "proof_payload_hex": proof_payload.hex(),
    })

    return vectors


def generate_resource_advertisement_vectors():
    """Generate resource advertisement format vectors."""
    from RNS.vendor import umsgpack

    vectors = []

    test_cases = [
        {
            "t": 1024, "d": 2048, "n": 3,
            "h": hashlib.sha256(b"resource-hash-data").digest(),
            "r": os.urandom(4) if False else hashlib.sha256(b"random-hash-seed").digest()[:4],
            "o": hashlib.sha256(b"original-hash-data").digest(),
            "i": 0, "l": 1, "q": b"",
            "f": 0x02,  # compressed only
            "m": hashlib.sha256(b"hashmap-data").digest()[:16],
        },
        {
            "t": 500000, "d": 1000000, "n": 130,
            "h": hashlib.sha256(b"large-resource-hash").digest(),
            "r": hashlib.sha256(b"large-random-hash").digest()[:4],
            "o": hashlib.sha256(b"large-original-hash").digest(),
            "i": 1, "l": 3, "q": hashlib.sha256(b"request-id").digest()[:16],
            "f": 0x1B,  # response + request + split + compressed + encrypted
            "m": hashlib.sha256(b"large-hashmap").digest()[:32],
        },
    ]

    for tc in test_cases:
        packed = umsgpack.packb(tc)
        unpacked = umsgpack.unpackb(packed)

        flags = tc["f"]
        vectors.append({
            "packed_hex": packed.hex(),
            "transfer_size": tc["t"],
            "data_size": tc["d"],
            "parts": tc["n"],
            "hash_hex": tc["h"].hex(),
            "random_hash_hex": tc["r"].hex(),
            "original_hash_hex": tc["o"].hex(),
            "segment_index": tc["i"],
            "total_segments": tc["l"],
            "request_id_hex": tc["q"].hex(),
            "flags": flags,
            "encrypted": bool(flags & 0x01),
            "compressed": bool(flags & 0x02),
            "split": bool(flags & 0x04),
            "is_request": bool(flags & 0x08),
            "is_response": bool(flags & 0x10),
            "has_metadata": bool(flags & 0x20),
            "hashmap_hex": tc["m"].hex(),
        })

    return vectors


def generate_channel_envelope_vectors():
    """Generate channel message envelope format vectors."""
    vectors = []

    test_cases = [
        (0x0001, 0, b"hello channel"),
        (0x0002, 100, b""),
        (0x1234, 0xFFFF, b"\x00" * 100),
        (0x0001, 42, b"channel message with some data for testing"),
    ]

    for msgtype, sequence, data in test_cases:
        # Envelope: [msgtype:2][sequence:2][length:2][data]
        envelope = struct.pack(">HHH", msgtype, sequence, len(data)) + data

        vectors.append({
            "envelope_hex": envelope.hex(),
            "msgtype": msgtype,
            "sequence": sequence,
            "length": len(data),
            "data_hex": data.hex(),
        })

    return vectors


def generate_buffer_stream_vectors():
    """Generate buffer stream data message format vectors."""
    vectors = []

    test_cases = [
        (0, False, False, b"stream data"),
        (1, True, False, b"final chunk"),
        (0x3FFF, False, True, b"compressed"),
        (42, True, True, b""),
        (100, False, False, b"\xff" * 50),
    ]

    for stream_id, eof, compressed, data in test_cases:
        header_val = (stream_id & 0x3FFF)
        if eof:
            header_val |= 0x8000
        if compressed:
            header_val |= 0x4000

        packed = struct.pack(">H", header_val) + data

        vectors.append({
            "packed_hex": packed.hex(),
            "stream_id": stream_id,
            "eof": eof,
            "compressed": compressed,
            "data_hex": data.hex(),
        })

    return vectors


def generate_resource_hash_vectors():
    """Generate resource hash computation vectors."""
    vectors = []

    test_data = [
        b"small resource data",
        b"A" * 1000,
        bytes(range(256)) * 4,
    ]

    for data in test_data:
        random_hash = hashlib.sha256(b"resource-random-" + data[:16]).digest()[:4]

        resource_hash = hashlib.sha256(data + random_hash).digest()
        truncated_hash = resource_hash[:16]
        proof_hash = hashlib.sha256(data + resource_hash).digest()

        # Map hash per part
        map_hash = hashlib.sha256(data[:384] + random_hash).digest()[:4]

        vectors.append({
            "data_hex": data.hex(),
            "random_hash_hex": random_hash.hex(),
            "resource_hash_hex": resource_hash.hex(),
            "truncated_hash_hex": truncated_hash.hex(),
            "proof_hash_hex": proof_hash.hex(),
            "map_hash_hex": map_hash.hex(),
            "maphash_len": 4,
        })

    return vectors


def generate_link_encryption_vectors():
    """Generate link encryption format vectors (Token format over link)."""
    vectors = []

    key = bytes(range(64))  # 64-byte derived key for AES-256-CBC
    signing_key = key[:32]
    encryption_key = key[32:64]

    test_data = [
        b"link encrypted payload",
        b"x" * 100,
        b"\x00",
    ]

    from RNS.Cryptography.PKCS7 import PKCS7
    from RNS.Cryptography.AES import AES_256_CBC

    for data in test_data:
        iv = hashlib.sha256(b"deterministic-iv-" + data[:8]).digest()[:16]
        padded = PKCS7.pad(data)
        ciphertext = AES_256_CBC.encrypt(plaintext=padded, key=encryption_key, iv=iv)
        signed_parts = iv + ciphertext
        mac = HMAC(signing_key, signed_parts).digest()
        token = signed_parts + mac

        vectors.append({
            "derived_key_hex": key.hex(),
            "plaintext_hex": data.hex(),
            "iv_hex": iv.hex(),
            "ciphertext_hex": ciphertext.hex(),
            "hmac_hex": mac.hex(),
            "token_hex": token.hex(),
        })

    return vectors


def generate_path_request_vectors():
    """Generate path request packet data format vectors."""
    vectors = []

    dest_hash = hashlib.sha256(b"path-request-dest").digest()[:16]
    requestor_id = hashlib.sha256(b"path-requestor-transport").digest()[:16]
    tag = hashlib.sha256(b"path-request-tag").digest()[:8]

    # dest_hash only (minimal)
    data1 = dest_hash
    vectors.append({
        "data_hex": data1.hex(),
        "dest_hash_hex": dest_hash.hex(),
        "requestor_id_hex": "",
        "tag_hex": "",
    })

    # dest_hash + tag (no requestor)
    data2 = dest_hash + tag
    vectors.append({
        "data_hex": data2.hex(),
        "dest_hash_hex": dest_hash.hex(),
        "requestor_id_hex": "",
        "tag_hex": tag.hex(),
    })

    # dest_hash + requestor_id + tag (full)
    data3 = dest_hash + requestor_id + tag
    vectors.append({
        "data_hex": data3.hex(),
        "dest_hash_hex": dest_hash.hex(),
        "requestor_id_hex": requestor_id.hex(),
        "tag_hex": tag.hex(),
    })

    return vectors


def generate_receipt_proof_vectors():
    """Generate packet receipt proof format vectors (explicit: hash + signature)."""
    vectors = []

    seed = hashlib.sha512(b"reticulum-crossref-test-seed-0").digest()
    identity = Identity(create_keys=False)
    identity.load_private_key(seed[:64])

    packet_hash = hashlib.sha256(b"packet to be proved").digest()
    signature = identity.sign(packet_hash)

    # Explicit proof: proof_hash(32) + signature(64) = 96 bytes
    proof = packet_hash + signature

    vectors.append({
        "packet_hash_hex": packet_hash.hex(),
        "signature_hex": signature.hex(),
        "proof_hex": proof.hex(),
        "public_key_hex": identity.get_public_key().hex(),
        "expl_length": 96,
    })

    return vectors


def generate_lrproof_packet_vectors():
    """Generate full LRPROOF packet layout vectors."""
    vectors = []

    link_id = hashlib.sha256(b"lrproof-link-id").digest()[:16]
    proof_payload = hashlib.sha256(b"proof-payload").digest()[:99]

    # LRPROOF: HEADER_1, dest_type=LINK, packet_type=PROOF, context=0xFF
    # Header: flags, hops, link_id (as dest_hash), context 0xFF, data
    flags = (0x00 << 6) | (0x00 << 5) | (0x00 << 4) | (0x03 << 2) | 0x03
    hops = 0
    context = 0xFF

    raw = struct.pack("!BB", flags, hops) + link_id + struct.pack("!B", context) + proof_payload

    vectors.append({
        "raw_hex": raw.hex(),
        "link_id_hex": link_id.hex(),
        "context": context,
        "proof_payload_hex": proof_payload.hex(),
        "header_type": 0,
        "packet_type": 3,
        "destination_type": 3,
    })

    return vectors


def generate_python_packet_vectors():
    """Generate full packet wire format using Python Packet.pack()."""
    vectors = []

    from RNS.Packet import Packet
    from RNS.Destination import Destination

    dest = Destination(None, Destination.OUT, Destination.PLAIN, "testapp")
    dest_hash = dest.hash

    # DATA packet (PLAIN dest = no encryption)
    pkt1 = Packet(dest, b"hello from python packet", create_receipt=False)
    pkt1.pack()

    vectors.append({
        "raw_hex": pkt1.raw.hex(),
        "packet_type": 0,
        "header_type": 0,
        "dest_hash_hex": dest_hash.hex(),
        "data_hex": b"hello from python packet".hex(),
    })

    # ANNOUNCE packet (plain, no encryption)
    pkt2 = Packet(dest, b"x" * 170, create_receipt=False)
    pkt2.packet_type = Packet.ANNOUNCE
    pkt2.flags = pkt2.get_packed_flags()
    pkt2.pack()

    vectors.append({
        "raw_hex": pkt2.raw.hex(),
        "packet_type": 1,
        "header_type": 0,
        "dest_hash_hex": dest_hash.hex(),
        "data_len": 170,
    })

    return vectors


def generate_resource_context_vectors():
    """Generate resource context constant values."""
    from RNS.Packet import Packet

    return [
        {"name": "RESOURCE", "value": Packet.RESOURCE},
        {"name": "RESOURCE_ADV", "value": Packet.RESOURCE_ADV},
        {"name": "RESOURCE_REQ", "value": Packet.RESOURCE_REQ},
        {"name": "RESOURCE_HMU", "value": Packet.RESOURCE_HMU},
        {"name": "RESOURCE_PRF", "value": Packet.RESOURCE_PRF},
        {"name": "RESOURCE_ICL", "value": Packet.RESOURCE_ICL},
        {"name": "RESOURCE_RCL", "value": Packet.RESOURCE_RCL},
        {"name": "LRPROOF", "value": Packet.LRPROOF},
    ]


def generate_resource_metadata_prefix_vectors():
    """Generate resource metadata prefix format: 3-byte size + msgpack."""
    from RNS.vendor import umsgpack

    vectors = []

    test_metadata = [
        {"key": "value"},
        {"size": 1024, "name": "test"},
        {},
    ]

    for meta in test_metadata:
        packed = umsgpack.packb(meta)
        metadata_size = len(packed)
        prefix = struct.pack(">I", metadata_size)[1:]
        full = prefix + packed

        vectors.append({
            "metadata_hex": packed.hex(),
            "prefix_hex": prefix.hex(),
            "full_hex": full.hex(),
            "metadata_size": metadata_size,
        })

    return vectors


def generate_buffer_compressed_vectors():
    """Generate buffer stream with bzip2-compressed data."""
    import bz2

    vectors = []

    data = b"repeated data for compression " * 20
    compressed = bz2.compress(data)

    header_val = (1 & 0x3FFF) | 0x4000
    packed = struct.pack(">H", header_val) + compressed

    vectors.append({
        "packed_hex": packed.hex(),
        "stream_id": 1,
        "compressed": True,
        "eof": False,
        "original_data_hex": data.hex(),
        "compressed_hex": compressed.hex(),
    })

    return vectors


def generate_resource_req_vectors():
    """Generate resource request packet data format vectors."""
    from RNS.Resource import Resource

    vectors = []

    resource_hash = hashlib.sha256(b"resource-req-hash-data").digest()
    map_hash_a = hashlib.sha256(b"map-hash-a").digest()[:4]
    map_hash_b = hashlib.sha256(b"map-hash-b").digest()[:4]
    last_map_hash = hashlib.sha256(b"last-map-hash").digest()[:4]

    hmu_part_normal = bytes([0x00])
    request_data_normal = hmu_part_normal + resource_hash + map_hash_a + map_hash_b
    vectors.append({
        "data_hex": request_data_normal.hex(),
        "hmu_part_hex": hmu_part_normal.hex(),
        "hashmap_exhausted": False,
        "resource_hash_hex": resource_hash.hex(),
        "last_map_hash_hex": "",
        "requested_hashes_hex": (map_hash_a + map_hash_b).hex(),
    })

    hmu_part_exhausted = bytes([Resource.HASHMAP_IS_EXHAUSTED])
    request_data_exhausted = hmu_part_exhausted + last_map_hash + resource_hash
    vectors.append({
        "data_hex": request_data_exhausted.hex(),
        "hmu_part_hex": hmu_part_exhausted.hex(),
        "hashmap_exhausted": True,
        "resource_hash_hex": resource_hash.hex(),
        "last_map_hash_hex": last_map_hash.hex(),
        "requested_hashes_hex": "",
    })

    return vectors


def generate_resource_hmu_vectors():
    """Generate resource hashmap update packet format vectors."""
    from RNS.vendor import umsgpack

    vectors = []

    resource_hash = hashlib.sha256(b"resource-hmu-hash").digest()
    segment = 0
    hashmap = hashlib.sha256(b"hashmap-seg-0-a").digest()[:4] + hashlib.sha256(b"hashmap-seg-0-b").digest()[:4]

    hmu = resource_hash + umsgpack.packb([segment, hashmap])
    vectors.append({
        "data_hex": hmu.hex(),
        "resource_hash_hex": resource_hash.hex(),
        "segment": segment,
        "hashmap_hex": hashmap.hex(),
    })

    segment2 = 2
    hashmap2 = hashlib.sha256(b"hm2-a").digest()[:4] * 8
    hmu2 = resource_hash + umsgpack.packb([segment2, hashmap2])
    vectors.append({
        "data_hex": hmu2.hex(),
        "resource_hash_hex": resource_hash.hex(),
        "segment": segment2,
        "hashmap_hex": hashmap2.hex(),
    })

    return vectors


def generate_resource_prf_vectors():
    """Generate resource proof packet format vectors."""
    vectors = []

    data = b"resource data for proof"
    resource_hash = hashlib.sha256(b"resource-prf-hash").digest()
    proof = hashlib.sha256(data + resource_hash).digest()
    proof_data = resource_hash + proof

    vectors.append({
        "data_hex": data.hex(),
        "resource_hash_hex": resource_hash.hex(),
        "proof_hex": proof.hex(),
        "proof_data_hex": proof_data.hex(),
    })

    data2 = b""
    resource_hash2 = hashlib.sha256(b"empty-resource").digest()
    proof2 = hashlib.sha256(data2 + resource_hash2).digest()
    proof_data2 = resource_hash2 + proof2
    vectors.append({
        "data_hex": data2.hex(),
        "resource_hash_hex": resource_hash2.hex(),
        "proof_hex": proof2.hex(),
        "proof_data_hex": proof_data2.hex(),
    })

    return vectors


def generate_resource_icl_rcl_vectors():
    """Generate resource initiator/receiver cancel packet format vectors."""
    vectors = []

    resource_hash = hashlib.sha256(b"resource-cancel-hash").digest()
    vectors.append({
        "payload_hex": resource_hash.hex(),
        "resource_hash_hex": resource_hash.hex(),
    })

    return vectors


def generate_lrrtt_vectors():
    """Generate LRRTT packet payload format vectors (msgpack float)."""
    from RNS.vendor import umsgpack

    vectors = []

    for rtt in [0.0, 0.123, 1.5, 42.0]:
        payload = umsgpack.packb(rtt)
        vectors.append({
            "payload_hex": payload.hex(),
            "rtt": rtt,
        })

    return vectors


def generate_destination_type_vectors():
    """Generate destination type constant values."""
    from RNS.Destination import Destination

    return [
        {"name": "SINGLE", "value": Destination.SINGLE},
        {"name": "GROUP", "value": Destination.GROUP},
        {"name": "PLAIN", "value": Destination.PLAIN},
        {"name": "LINK", "value": Destination.LINK},
    ]


def generate_cache_request_vectors():
    """Generate cache request packet format vectors."""
    vectors = []

    packet_hash = hashlib.sha256(b"packet-to-cache-request").digest()
    vectors.append({
        "payload_hex": packet_hash.hex(),
        "packet_hash_hex": packet_hash.hex(),
        "context": 0x08,
    })

    return vectors


def main():
    all_vectors = {
        "format_version": 5,
        "generator": "Python Reticulum reference implementation",
        "identity": generate_identity_vectors(),
        "destination_hash": generate_destination_hash_vectors(),
        "hkdf": generate_hkdf_vectors(),
        "hmac": generate_hmac_vectors(),
        "token": generate_token_vectors(),
        "packet_header": generate_packet_header_vectors(),
        "announce": generate_announce_vectors(),
        "encryption": generate_encryption_vectors(),
        "hash": generate_hash_vectors(),
        "ecdh": generate_ecdh_vectors(),
        "ratchet_id": generate_ratchet_id_vectors(),
        "packet_wire": generate_packet_wire_vectors(),
        "packet_hash": generate_packet_hash_vectors(),
        "aes": generate_aes_vectors(),
        "cross_sign": generate_cross_sign_vectors(),
        "identity_file": generate_identity_file_vectors(),
        "link_signalling": generate_link_signalling_vectors(),
        "link_key_derivation": generate_link_key_derivation_vectors(),
        "link_request": generate_link_request_vectors(),
        "link_proof": generate_link_proof_vectors(),
        "resource_advertisement": generate_resource_advertisement_vectors(),
        "channel_envelope": generate_channel_envelope_vectors(),
        "buffer_stream": generate_buffer_stream_vectors(),
        "resource_hash": generate_resource_hash_vectors(),
        "link_encryption": generate_link_encryption_vectors(),
        "path_request": generate_path_request_vectors(),
        "receipt_proof": generate_receipt_proof_vectors(),
        "lrproof_packet": generate_lrproof_packet_vectors(),
        "python_packet": generate_python_packet_vectors(),
        "resource_context": generate_resource_context_vectors(),
        "resource_metadata_prefix": generate_resource_metadata_prefix_vectors(),
        "buffer_compressed": generate_buffer_compressed_vectors(),
        "resource_req": generate_resource_req_vectors(),
        "resource_hmu": generate_resource_hmu_vectors(),
        "resource_prf": generate_resource_prf_vectors(),
        "resource_icl_rcl": generate_resource_icl_rcl_vectors(),
        "lrrtt": generate_lrrtt_vectors(),
        "destination_type": generate_destination_type_vectors(),
        "cache_request": generate_cache_request_vectors(),
    }

    output_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "test_vectors.json")
    with open(output_path, "w") as f:
        json.dump(all_vectors, f, indent=2)

    print(f"Generated test vectors: {output_path}")
    for section, vectors in all_vectors.items():
        if isinstance(vectors, list):
            print(f"  {section}: {len(vectors)} vectors")


if __name__ == "__main__":
    main()
