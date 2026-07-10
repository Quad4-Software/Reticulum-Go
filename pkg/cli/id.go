// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/rnsutil"
)

const defaultAspects = "rns.id"

func RunID(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgoid", flag.ContinueOnError)
	fs.SetOutput(stderr)

	identityPath := fs.String("i", "", "path to identity file or hex identity hash")
	generate := fs.String("g", "", "generate identity and write to path")
	importPub := fs.String("m", "", "import public key (hex/base64/base32)")
	importPrv := fs.String("M", "", "import private key (hex/base64/base32)")
	exportPub := fs.Bool("x", false, "export public key")
	exportPrv := fs.Bool("X", false, "export private key")
	printID := fs.Bool("p", false, "print identity hash and keys summary")
	printPriv := fs.Bool("P", false, "allow printing private key material")
	hashAspects := fs.String("H", "", "print destination hash for dotted aspects name")
	signPath := fs.String("s", "", "sign file to .rsg")
	signMsg := fs.String("S", "", "create embedded signed message (.rsm)")
	verifyPath := fs.String("V", "", "validate .rsg/.rsm or file+signature")
	encryptPath := fs.String("e", "", "encrypt file to .rfe")
	decryptPath := fs.String("d", "", "decrypt .rfe file")
	rawSign := fs.Bool("raw", false, "write legacy raw Ed25519 signature only")
	showMeta := fs.Bool("meta", false, "display RSM metadata when validating")
	writeOut := fs.String("w", "", "write output to path")
	force := fs.Bool("f", false, "overwrite existing output")
	useB64 := fs.Bool("b", false, "base64 encoding")
	useB32 := fs.Bool("B", false, "base32 encoding")
	useHex := fs.Bool("hex", false, "hex encoding (default)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	enc, err := pickEncoding(*useB64, *useB32, *useHex)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	ident, err := resolveIdentity(*identityPath, *generate, *importPub, *importPrv, enc)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	didWork := false
	if *generate != "" {
		didWork = true
		fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("Generated identity %s", ident.GetHexHash())))
	}
	if *printID {
		if ident == nil {
			fmt.Fprintln(stderr, "no identity")
			return 1
		}
		didWork = true
		printIdentity(ident, *printPriv, enc)
	}
	if *exportPub {
		if ident == nil {
			fmt.Fprintln(stderr, "no identity")
			return 1
		}
		didWork = true
		out := rnsutil.EncodeBytes(ident.GetPublicKey(), enc) + "\n"
		if err := writeOutput(*writeOut, []byte(out), *force); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
	}
	if *exportPrv {
		if ident == nil {
			fmt.Fprintln(stderr, "no identity")
			return 1
		}
		didWork = true
		priv, err := ident.GetPrivateKey()
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		out := rnsutil.EncodeBytes(priv, enc) + "\n"
		if err := writeOutput(*writeOut, []byte(out), *force); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
	}
	if *hashAspects != "" {
		if ident == nil {
			fmt.Fprintln(stderr, "hash requires an identity")
			return 1
		}
		didWork = true
		name := *hashAspects
		if name == "" {
			name = defaultAspects
		}
		h, err := rnsutil.DestinationHashHex(ident, name)
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, h)
	}
	if *signPath != "" {
		if ident == nil {
			fmt.Fprintln(stderr, "signing requires an identity")
			return 1
		}
		didWork = true
		if code := doSign(ident, *signPath, *writeOut, *force, *rawSign); code != 0 {
			return code
		}
	}
	if *signMsg != "" {
		if ident == nil {
			fmt.Fprintln(stderr, "signing requires an identity")
			return 1
		}
		didWork = true
		if code := doSignMessage(ident, *signMsg, *writeOut, *force); code != 0 {
			return code
		}
	}
	if *verifyPath != "" {
		didWork = true
		if code := doVerify(ident, *identityPath, *verifyPath, *showMeta); code != 0 {
			return code
		}
	}
	if *encryptPath != "" {
		if ident == nil {
			fmt.Fprintln(stderr, "encrypt requires an identity")
			return 1
		}
		didWork = true
		if code := doEncrypt(ident, *encryptPath, *writeOut, *force); code != 0 {
			return code
		}
	}
	if *decryptPath != "" {
		if ident == nil {
			fmt.Fprintln(stderr, "decrypt requires an identity")
			return 1
		}
		didWork = true
		if code := doDecrypt(ident, *decryptPath, *writeOut, *force); code != 0 {
			return code
		}
	}

	if !didWork {
		fs.Usage()
		return 2
	}
	return 0
}

func readInputBytes(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path) // #nosec G304
}

func doSign(ident *identity.Identity, signPath, writeOut string, force, raw bool) int {
	signPath = expand(signPath)
	outPath := writeOut
	if outPath == "" {
		outPath = signPath + "." + rnsutil.SigExt
	}
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			fmt.Fprintf(os.Stderr, "refusing to overwrite %s (use -f)\n", outPath)
			return 11
		}
	}
	var data []byte
	var err error
	if raw {
		payload, err := readInputBytes(signPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 6
		}
		data, err = ident.Sign(payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 254
		}
	} else {
		data, err = rnsutil.SignFileRSG(ident, signPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 254
		}
	}
	if err := rnsutil.WriteFileAtomic(outPath, data); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 253
	}
	fmt.Fprintln(os.Stdout, okMsg(os.Stdout, fmt.Sprintf("Signed file %s with %s", signPath, ident.GetHexHash())))
	return 0
}

func doSignMessage(ident *identity.Identity, message, writeOut string, force bool) int {
	if writeOut == "" {
		fmt.Fprintln(os.Stderr, "signed message requires -w path")
		return 250
	}
	outPath := expand(writeOut)
	if !strings.HasSuffix(strings.ToLower(outPath), "."+rnsutil.MsgExt) {
		outPath += "." + rnsutil.MsgExt
	}
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			fmt.Fprintf(os.Stderr, "refusing to overwrite %s (use -f)\n", outPath)
			return 11
		}
	}
	rsm, err := rnsutil.CreateRSM(ident, message, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 254
	}
	if err := rnsutil.WriteFileAtomic(outPath, rsm); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 253
	}
	fmt.Fprintln(os.Stdout, okMsg(os.Stdout, fmt.Sprintf("Message signed with %s saved to %s", ident.GetHexHash(), outPath)))
	return 0
}

func doVerify(ident *identity.Identity, identityArg, verifyPath string, showMeta bool) int {
	verifyPath = expand(verifyPath)
	lower := strings.ToLower(verifyPath)
	switch {
	case strings.HasSuffix(lower, "."+rnsutil.MsgExt):
		rsm, err := os.ReadFile(verifyPath) // #nosec G304
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 6
		}
		var required any
		if ident != nil {
			required = ident
		} else if len(identityArg) == 32 {
			required = identityArg
		}
		res, text, err := rnsutil.VerifyRSM(rsm, required)
		if err != nil || !res.Valid {
			fmt.Fprintln(os.Stderr, errMsg(os.Stderr, fmt.Sprintf("Invalid signature in %s", verifyPath)))
			return 10
		}
		if showMeta && res.Envelope != nil && res.Envelope.Meta != nil {
			fmt.Fprintln(os.Stdout, infoMsg(os.Stdout, "RSM Metadata"))
			for k, v := range res.Envelope.Meta {
				fmt.Fprintf(os.Stdout, "  %s=%v\n", k, v)
			}
		}
		signer := res.Signer.GetHexHash()
		fmt.Fprintf(os.Stdout, "%s, the message was signed by %s\n\n%s\n", okMsg(os.Stdout, "Signature is valid"), signer, text)
		return 0
	case strings.HasSuffix(lower, "."+rnsutil.SigExt):
		filePath := verifyPath[:len(verifyPath)-len(rnsutil.SigExt)-1]
		rsg, err := os.ReadFile(verifyPath) // #nosec G304
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 6
		}
		var required any
		if ident != nil {
			required = ident
		}
		res, err := rnsutil.VerifyFileRSG(rsg, filePath, required)
		if err != nil || !res.Valid {
			fmt.Fprintln(os.Stderr, errMsg(os.Stderr, fmt.Sprintf("Invalid signature %s for file %s", verifyPath, filePath)))
			return 10
		}
		fmt.Fprintf(os.Stdout, "%s, the file %s was signed by %s\n", okMsg(os.Stdout, "Signature is valid"), filePath, res.Signer.GetHexHash())
		return 0
	default:
		rsgPath := verifyPath + "." + rnsutil.SigExt
		rsg, err := os.ReadFile(rsgPath) // #nosec G304
		if err != nil {
			fmt.Fprintf(os.Stderr, "No signature file exists for %q\n", verifyPath)
			return 6
		}
		var required any
		if ident != nil {
			required = ident
		}
		res, err := rnsutil.VerifyFileRSG(rsg, verifyPath, required)
		if err != nil || !res.Valid {
			fmt.Fprintln(os.Stderr, errMsg(os.Stderr, fmt.Sprintf("Invalid signature %s for file %s", rsgPath, verifyPath)))
			return 10
		}
		fmt.Fprintf(os.Stdout, "%s, the file %s was signed by %s\n", okMsg(os.Stdout, "Signature is valid"), verifyPath, res.Signer.GetHexHash())
		return 0
	}
}

func doEncrypt(ident *identity.Identity, encryptPath, writeOut string, force bool) int {
	encryptPath = expand(encryptPath)
	outPath := writeOut
	if outPath == "" {
		outPath = encryptPath + "." + rnsutil.EncryptExt
	} else {
		outPath = expand(outPath)
	}
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			fmt.Fprintf(os.Stderr, "refusing to overwrite %s (use -f)\n", outPath)
			return 11
		}
	}
	if err := rnsutil.EncryptFileRFE(ident, encryptPath, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 254
	}
	fmt.Fprintln(os.Stdout, okMsg(os.Stdout, fmt.Sprintf("File %s encrypted for %s to %s", encryptPath, ident.GetHexHash(), outPath)))
	return 0
}

func doDecrypt(ident *identity.Identity, decryptPath, writeOut string, force bool) int {
	decryptPath = expand(decryptPath)
	if !strings.HasSuffix(strings.ToLower(decryptPath), "."+rnsutil.EncryptExt) {
		fmt.Fprintf(os.Stderr, "The file %s does not appear to be a Reticulum encrypted file\n", decryptPath)
		return 7
	}
	outPath := writeOut
	if outPath == "" {
		outPath = decryptPath[:len(decryptPath)-len(rnsutil.EncryptExt)-1]
	} else {
		outPath = expand(outPath)
	}
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			fmt.Fprintf(os.Stderr, "refusing to overwrite %s (use -f)\n", outPath)
			return 11
		}
	}
	if err := rnsutil.DecryptFileRFE(ident, decryptPath, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 12
	}
	fmt.Fprintln(os.Stdout, okMsg(os.Stdout, fmt.Sprintf("File %s decrypted to %s", decryptPath, outPath)))
	return 0
}

func pickEncoding(b64, b32, hexFlag bool) (rnsutil.Encoding, error) {
	n := 0
	enc := rnsutil.EncHex
	if b64 {
		enc = rnsutil.EncBase64
		n++
	}
	if b32 {
		enc = rnsutil.EncBase32
		n++
	}
	if hexFlag {
		enc = rnsutil.EncHex
		n++
	}
	if n > 1 {
		return enc, fmt.Errorf("encoding flags -b, -B, and -hex are mutually exclusive")
	}
	return enc, nil
}

func resolveIdentity(path, generate, importPub, importPrv string, enc rnsutil.Encoding) (*identity.Identity, error) {
	n := 0
	if path != "" {
		n++
	}
	if generate != "" {
		n++
	}
	if importPub != "" {
		n++
	}
	if importPrv != "" {
		n++
	}
	if n > 1 {
		return nil, fmt.Errorf("-i, -g, -m and -M are mutually exclusive")
	}
	switch {
	case generate != "":
		return rnsutil.GenerateIdentity(expand(generate))
	case importPub != "":
		if st, err := os.Stat(expand(importPub)); err == nil && !st.IsDir() {
			b, err := os.ReadFile(expand(importPub)) // #nosec G304
			if err != nil {
				return nil, err
			}
			if len(b) == 64 {
				id := identity.FromPublicKey(b)
				if id == nil {
					return nil, fmt.Errorf("invalid public key file")
				}
				return id, nil
			}
			return rnsutil.ImportPublicIdentity(string(bytesTrim(b)), enc)
		}
		return rnsutil.ImportPublicIdentity(importPub, enc)
	case importPrv != "":
		if st, err := os.Stat(expand(importPrv)); err == nil && !st.IsDir() {
			return rnsutil.LoadIdentity(expand(importPrv))
		}
		return rnsutil.ImportPrivateIdentity(importPrv, enc)
	case path != "":
		p := expand(path)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return rnsutil.LoadIdentity(p)
		}
		return nil, fmt.Errorf("identity file not found: %s", p)
	default:
		return nil, nil
	}
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func printIdentity(id *identity.Identity, showPriv bool, enc rnsutil.Encoding) {
	fmt.Fprintf(os.Stdout, "%s : %s\n", infoMsg(os.Stdout, "Identity hash"), id.GetHexHash())
	fmt.Fprintf(os.Stdout, "%s : %s\n", infoMsg(os.Stdout, "Public key   "), rnsutil.EncodeBytes(id.GetPublicKey(), enc))
	if !showPriv {
		return
	}
	priv, err := id.GetPrivateKey()
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s : unavailable (%v)\n", warnMsg(os.Stdout, "Private key  "), err)
		return
	}
	fmt.Fprintf(os.Stdout, "%s : %s\n", infoMsg(os.Stdout, "Private key  "), rnsutil.EncodeBytes(priv, enc))
}

func writeOutput(path string, data []byte, force bool) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	path = expand(path)
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("refusing to overwrite %s (use -f)", path)
		}
	}
	return rnsutil.WriteFileAtomic(path, data)
}

func expand(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
