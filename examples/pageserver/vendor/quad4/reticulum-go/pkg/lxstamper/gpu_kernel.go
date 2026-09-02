// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build ((linux && !android && (amd64 || arm64 || loong64)) || (darwin && (amd64 || arm64))) && lxstamp_gpu

package lxstamper

// OpenCL program: stamp search, parallel StampWorkblock HKDF, and batch validation.
// Targets NVIDIA / AMD / Intel via the platform ICD loader.
const lxstampOpenCLKernel = `
#define ROTR(x,n) (rotate((uint)(x), (uint)(32-(n))))
#define Ch(x,y,z)  (((x)&(y)) ^ (~(x)&(z)))
#define Maj(x,y,z) (((x)&(y)) ^ ((x)&(z)) ^ ((y)&(z)))
#define S0(x) (ROTR(x,2) ^ ROTR(x,13) ^ ROTR(x,22))
#define S1(x) (ROTR(x,6) ^ ROTR(x,11) ^ ROTR(x,25))
#define s0(x) (ROTR(x,7) ^ ROTR(x,18) ^ ((x)>>3))
#define s1(x) (ROTR(x,17) ^ ROTR(x,19) ^ ((x)>>10))

__constant uint K[64] = {
  0x428a2f98U,0x71374491U,0xb5c0fbcfU,0xe9b5dba5U,0x3956c25bU,0x59f111f1U,0x923f82a4U,0xab1c5ed5U,
  0xd807aa98U,0x12835b01U,0x243185beU,0x550c7dc3U,0x72be5d74U,0x80deb1feU,0x9bdc06a7U,0xc19bf174U,
  0xe49b69c1U,0xefbe4786U,0x0fc19dc6U,0x240ca1ccU,0x2de92c6fU,0x4a7484aaU,0x5cb0a9dcU,0x76f988daU,
  0x983e5152U,0xa831c66dU,0xb00327c8U,0xbf597fc7U,0xc6e00bf3U,0xd5a79147U,0x06ca6351U,0x14292967U,
  0x27b70a85U,0x2e1b2138U,0x4d2c6dfcU,0x53380d13U,0x650a7354U,0x766a0abbU,0x81c2c92eU,0x92722c85U,
  0xa2bfe8a1U,0xa81a664bU,0xc24b8b70U,0xc76c51a3U,0xd192e819U,0xd6990624U,0xf40e3585U,0x106aa070U,
  0x19a4c116U,0x1e376c08U,0x2748774cU,0x34b0bcb5U,0x391c0cb3U,0x4ed8aa4aU,0x5b9cca4fU,0x682e6ff3U,
  0x748f82eeU,0x78a5636fU,0x84c87814U,0x8cc70208U,0x90befffaU,0xa4506cebU,0xbef9a3f7U,0xc67178f2U
};

void compress(uint *S, const uchar *block) {
  uint W[64];
  for (int i = 0; i < 16; i++) {
    int j = i*4;
    W[i] = ((uint)block[j]<<24) | ((uint)block[j+1]<<16) | ((uint)block[j+2]<<8) | (uint)block[j+3];
  }
  for (int i = 16; i < 64; i++) {
    W[i] = s1(W[i-2]) + W[i-7] + s0(W[i-15]) + W[i-16];
  }
  uint a=S[0],b=S[1],c=S[2],d=S[3],e=S[4],f=S[5],g=S[6],h=S[7];
  for (int i = 0; i < 64; i++) {
    uint t1 = h + S1(e) + Ch(e,f,g) + K[i] + W[i];
    uint t2 = S0(a) + Maj(a,b,c);
    h=g; g=f; f=e; e=d+t1; d=c; c=b; b=a; a=t1+t2;
  }
  S[0]+=a; S[1]+=b; S[2]+=c; S[3]+=d; S[4]+=e; S[5]+=f; S[6]+=g; S[7]+=h;
}

void sha256_init(uint *S) {
  S[0]=0x6a09e667U; S[1]=0xbb67ae85U; S[2]=0x3c6ef372U; S[3]=0xa54ff53aU;
  S[4]=0x510e527fU; S[5]=0x9b05688cU; S[6]=0x1f83d9abU; S[7]=0x5be0cd19U;
}

void sha256_finalize(uint *S, uchar *buf, uint n, ulong bitlen_total, uchar *out32) {
  uint off = 0;
  while (n - off >= 64u) {
    compress(S, buf + off);
    off += 64u;
  }
  uchar block[64];
  uint remn = n - off;
  for (uint i = 0; i < remn; i++) block[i] = buf[off + i];
  block[remn] = (uchar)0x80;
  for (uint i = remn + 1u; i < 64u; i++) block[i] = 0;
  if (remn >= 56u) {
    compress(S, block);
    for (int i = 0; i < 56; i++) block[i] = 0;
  }
  for (int i = 0; i < 8; i++) block[63 - i] = (uchar)(bitlen_total >> (8 * i));
  compress(S, block);
  for (int i = 0; i < 8; i++) {
    out32[i*4]   = (uchar)(S[i] >> 24);
    out32[i*4+1] = (uchar)(S[i] >> 16);
    out32[i*4+2] = (uchar)(S[i] >> 8);
    out32[i*4+3] = (uchar)(S[i]);
  }
}

void sha256_oneshot(const uchar *msg, uint msg_len, uchar *out32) {
  uint S[8];
  sha256_init(S);
  uchar tmp[256];
  for (uint i = 0; i < msg_len; i++) tmp[i] = msg[i];
  sha256_finalize(S, tmp, msg_len, (ulong)msg_len * 8ul, out32);
}

void hmac_sha256(const uchar *key, uint key_len, const uchar *msg, uint msg_len, uchar *out32) {
  uchar kbuf[64];
  for (int i = 0; i < 64; i++) kbuf[i] = 0;
  if (key_len > 64u) {
    sha256_oneshot(key, key_len, kbuf);
  } else {
    for (uint i = 0; i < key_len; i++) kbuf[i] = key[i];
  }
  uchar ipad[64], opad[64];
  for (int i = 0; i < 64; i++) {
    ipad[i] = kbuf[i] ^ (uchar)0x36;
    opad[i] = kbuf[i] ^ (uchar)0x5c;
  }
  uint Si[8];
  sha256_init(Si);
  compress(Si, ipad);
  uchar inner_buf[320];
  for (uint i = 0; i < msg_len; i++) inner_buf[i] = msg[i];
  uchar inner[32];
  sha256_finalize(Si, inner_buf, msg_len, (ulong)(64u + msg_len) * 8ul, inner);

  uint So[8];
  sha256_init(So);
  compress(So, opad);
  uchar outer_buf[32];
  for (int i = 0; i < 32; i++) outer_buf[i] = inner[i];
  sha256_finalize(So, outer_buf, 32u, (ulong)(64u + 32u) * 8ul, out32);
}

void msgpack_uint(uint n, uchar *out, uint *olen) {
  if (n < 128u) { out[0] = (uchar)n; *olen = 1u; }
  else if (n < 256u) { out[0] = (uchar)0xcc; out[1] = (uchar)n; *olen = 2u; }
  else { out[0] = (uchar)0xcd; out[1] = (uchar)(n >> 8); out[2] = (uchar)(n & 0xffu); *olen = 3u; }
}

void hkdf_round_256(const uchar *material, uint material_len, uint round, uchar *out256) {
  uchar mp[4];
  uint mplen = 0;
  msgpack_uint(round, mp, &mplen);
  uchar salt_src[96];
  for (uint i = 0; i < material_len; i++) salt_src[i] = material[i];
  for (uint i = 0; i < mplen; i++) salt_src[material_len + i] = mp[i];
  uchar salt[32];
  sha256_oneshot(salt_src, material_len + mplen, salt);

  uchar prk[32];
  hmac_sha256(salt, 32u, material, material_len, prk);

  uchar block[32];
  for (int i = 0; i < 32; i++) block[i] = 0;
  uint block_len = 0;
  for (uint i = 0; i < 8u; i++) {
    uchar msg[33];
    for (uint j = 0; j < block_len; j++) msg[j] = block[j];
    msg[block_len] = (uchar)(i + 1u);
    hmac_sha256(prk, 32u, msg, block_len + 1u, block);
    block_len = 32u;
    for (int j = 0; j < 32; j++) out256[i * 32u + (uint)j] = block[j];
  }
}

__kernel void lxstamp_search(
  __constant uint *midstate,
  __constant uchar *rem,
  const uint rem_len,
  const ulong total_bitlen,
  __constant uchar *target,
  const ulong base,
  const ulong seed,
  __global volatile uint *found,
  __global uchar *out_stamp
) {
  if (*found != 0) return;
  ulong id = base + (ulong)get_global_id(0);
  uchar stamp[32];
  stamp[0] = (uchar)(id >> 56); stamp[1] = (uchar)(id >> 48);
  stamp[2] = (uchar)(id >> 40); stamp[3] = (uchar)(id >> 32);
  stamp[4] = (uchar)(id >> 24); stamp[5] = (uchar)(id >> 16);
  stamp[6] = (uchar)(id >> 8);  stamp[7] = (uchar)(id);
  stamp[8] = (uchar)(seed >> 56); stamp[9] = (uchar)(seed >> 48);
  stamp[10] = (uchar)(seed >> 40); stamp[11] = (uchar)(seed >> 32);
  stamp[12] = (uchar)(seed >> 24); stamp[13] = (uchar)(seed >> 16);
  stamp[14] = (uchar)(seed >> 8);  stamp[15] = (uchar)(seed);
  for (int i = 16; i < 32; i++) stamp[i] = (uchar)(id * 131u + (uint)i);

  uint S[8];
  for (int i = 0; i < 8; i++) S[i] = midstate[i];
  uchar msg[128];
  for (uint i = 0; i < rem_len; i++) msg[i] = rem[i];
  for (int i = 0; i < 32; i++) msg[rem_len + i] = stamp[i];
  uint n = rem_len + 32u;
  uint off = 0;
  while (n - off >= 64u) { compress(S, msg + off); off += 64u; }
  uchar block[64];
  uint remn = n - off;
  for (uint i = 0; i < remn; i++) block[i] = msg[off + i];
  block[remn] = (uchar)0x80;
  for (uint i = remn + 1u; i < 64u; i++) block[i] = 0;
  if (remn >= 56u) {
    compress(S, block);
    for (int i = 0; i < 56; i++) block[i] = 0;
  }
  for (int i = 0; i < 8; i++) block[63 - i] = (uchar)(total_bitlen >> (8 * i));
  compress(S, block);
  uchar hash[32];
  for (int i = 0; i < 8; i++) {
    hash[i*4] = (uchar)(S[i] >> 24); hash[i*4+1] = (uchar)(S[i] >> 16);
    hash[i*4+2] = (uchar)(S[i] >> 8); hash[i*4+3] = (uchar)(S[i]);
  }
  bool ok = true;
  for (int i = 0; i < 32; i++) {
    if (hash[i] < target[i]) break;
    if (hash[i] > target[i]) { ok = false; break; }
  }
  if (!ok) return;
  if (atomic_cmpxchg(found, 0u, 1u) == 0u) {
    for (int i = 0; i < 32; i++) out_stamp[i] = stamp[i];
  }
}

__kernel void lxstamp_workblock(
  __constant uchar *material,
  const uint material_len,
  const uint rounds,
  __global uchar *out
) {
  uint n = (uint)get_global_id(0);
  if (n >= rounds) return;
  uchar mat[64];
  for (uint i = 0; i < material_len; i++) mat[i] = material[i];
  uchar round_out[256];
  hkdf_round_256(mat, material_len, n, round_out);
  for (uint i = 0; i < 256u; i++) out[(ulong)n * 256ul + i] = round_out[i];
}

__kernel void lxstamp_batch_validate(
  __global const uchar *materials,
  __global const uint *material_lens,
  __global const uchar *stamps,
  const uint expand_rounds,
  __constant uchar *target,
  const uint cost,
  __global uchar *ok_out
) {
  uint gid = (uint)get_global_id(0);
  uint mlen = material_lens[gid];
  if (mlen == 0u || mlen > 64u) { ok_out[gid] = 0; return; }
  uchar mat[64];
  for (uint i = 0; i < mlen; i++) mat[i] = materials[(ulong)gid * 64ul + i];
  uchar stamp[32];
  for (int i = 0; i < 32; i++) stamp[i] = stamps[(ulong)gid * 32ul + (uint)i];

  uint S[8];
  sha256_init(S);
  ulong bytes = 0;
  for (uint r = 0; r < expand_rounds; r++) {
    uchar chunk[256];
    hkdf_round_256(mat, mlen, r, chunk);
    for (uint off = 0; off < 256u; off += 64u) {
      compress(S, chunk + off);
    }
    bytes += 256ul;
  }
  uchar fin[96];
  for (int i = 0; i < 32; i++) fin[i] = stamp[i];
  uchar hash[32];
  sha256_finalize(S, fin, 32u, (bytes + 32ul) * 8ul, hash);

  if (cost == 0u) {
    ok_out[gid] = 1;
    return;
  }
  bool thr_ok = true;
  for (int i = 0; i < 32; i++) {
    if (hash[i] < target[i]) break;
    if (hash[i] > target[i]) { thr_ok = false; break; }
  }
  int value = 0;
  for (int i = 0; i < 32; i++) {
    uchar b = hash[i];
    if (b == 0) { value += 8; continue; }
    for (int bit = 7; bit >= 0; bit--) {
      if ((b & (1 << bit)) != 0) { i = 32; break; }
      value++;
    }
    break;
  }
  ok_out[gid] = ( thr_ok && value >= (int)cost ) ? (uchar)1 : (uchar)0;
}
`
