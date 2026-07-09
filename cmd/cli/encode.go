package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

var encodeCmd = &cobra.Command{
	Use:   "encode TYPE VALUE",
	Short: "Encode value (url, base64, hex)",
	Args:  cobra.ExactArgs(2),
	RunE:  runEncode,
}

var decodeCmd = &cobra.Command{
	Use:   "decode TYPE VALUE",
	Short: "Decode value (url, base64, hex)",
	Args:  cobra.ExactArgs(2),
	RunE:  runDecode,
}

func runEncode(cmd *cobra.Command, args []string) error {
	typ := strings.ToLower(args[0])
	val := args[1]

	switch typ {
	case "url":
		fmt.Println(url.QueryEscape(val))
	case "base64", "b64":
		fmt.Println(base64.StdEncoding.EncodeToString([]byte(val)))
	case "hex":
		fmt.Println(hex.EncodeToString([]byte(val)))
	default:
		return fmt.Errorf(
			"unknown type %q: use url, base64, or hex", typ,
		)
	}
	return nil
}

func runDecode(cmd *cobra.Command, args []string) error {
	typ := strings.ToLower(args[0])
	val := args[1]

	switch typ {
	case "url":
		decoded, err := url.QueryUnescape(val)
		if err != nil {
			return fmt.Errorf("url decode: %w", err)
		}
		fmt.Println(decoded)
	case "base64", "b64":
		decoded, err := base64.StdEncoding.DecodeString(val)
		if err != nil {
			decoded, err = base64.URLEncoding.DecodeString(val)
			if err != nil {
				decoded, err = base64.RawStdEncoding.DecodeString(val)
				if err != nil {
					decoded, err = base64.RawURLEncoding.DecodeString(val)
					if err != nil {
						return fmt.Errorf("base64 decode: %w", err)
					}
				}
			}
		}
		fmt.Println(string(decoded))
	case "hex":
		decoded, err := hex.DecodeString(val)
		if err != nil {
			return fmt.Errorf("hex decode: %w", err)
		}
		fmt.Println(string(decoded))
	default:
		return fmt.Errorf(
			"unknown type %q: use url, base64, or hex", typ,
		)
	}
	return nil
}
