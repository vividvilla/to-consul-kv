// consul-cfg
//
// Commandline utility to convert config formats like TOML, JSON, YAML etc to KV Pairs which can be imported via consul cli.
// Output is same as JSON representation generated by the kv export command. Currently only toml is supported.
//
// # Read config from multiple files
// consul-cfg kv --type toml config1.toml config2.toml
//
// # Pipe stdin from other commands
// cat config.toml | consul-cfg kv --type toml
//
// # Specify prefix for all keys
// cat config.toml | consul-cfg kv --type toml --prefix myconfig/app

package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"reflect"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type consulKVPair struct {
	Key   string `json:"key"`
	Flags int    `json:"flags"`
	Value string `json:"value"`
}

var (
	kvInputType        string
	kvKeyPrefix        string
	kvAvailableFormats = []string{"toml", "yaml", "hcl", "json", "props"}

	sysLog = log.New(os.Stdout, "", log.LUTC)
	errLog = log.New(os.Stderr, "", log.LUTC)
)

func main() {
	// Configure CLI
	var rootCmd = &cobra.Command{
		Use:   "consul-cfg [sub]",
		Short: "Commandline utils for managing app configs with Consul",
		Long:  `consul-cfg is a set of utilities for managing app configurations with Consul like generating config template for consul template and exporting app config as consul KV JSON pair config which can be used to bulk import key pairs to Consul.`,
		Args:  cobra.MinimumNArgs(0),
	}

	// Configure CLI
	var kvCmd = &cobra.Command{
		Use:   "kv [file...]",
		Short: "Commandline utility to convert any config format to consul KV pairs format.",
		Long:  `consul-cfg is a command line utility to convert config formats like toml to JSON which can be imported to consul.`,
		Args:  cobra.MinimumNArgs(0),
		Run:   runKVCmd,
	}

	// Configure flags
	kvCmd.Flags().StringVarP(&kvInputType, "type", "t", "", "Input config format type. Available options are `toml`, `yaml`, `hcl`, `json` and `props` (JAVA properties)")
	kvCmd.Flags().StringVarP(&kvKeyPrefix, "prefix", "p", "", "Prefix for all keys")

	// Add sub command to root
	rootCmd.AddCommand(kvCmd)

	// Execute cli
	if err := rootCmd.Execute(); err != nil {
		errLog.Fatal(err)
	}
}

func runKVCmd(cmd *cobra.Command, args []string) {
	if !isValidKVInputFormat(kvInputType) {
		errLog.Fatalf("Invalid input file format - %s. Available options are `toml`, `yaml`, `hcl`, `json` and `props` (JAVA properties)", kvInputType)
	}

	// Collect all inputs
	var inputs []io.Reader
	var output []consulKVPair

	// Add stdin as default input if files are not provided
	if len(args) == 0 {
		inputs = append(inputs, os.Stdin)
	} else {
		// Add all files as inputs
		for _, fname := range args {
			f, err := os.Open(fname)
			if err != nil {
				errLog.Fatalf("Error: error opening input file - %v", err)
			}

			inputs = append(inputs, f)
		}
	}

	for _, i := range inputs {
		// Process toml inputs
		m, err := configToMap(kvInputType, i)

		// m, err := tomlToMap(i)
		if err != nil {
			errLog.Fatalf("Error: error parsing input - %v", err)
		}

		mapToKVPairs(&output, kvKeyPrefix, m)
	}

	// Print JSON output
	printKVPairsJSON(output)
}

// Check if given input format is supported.
func isValidKVInputFormat(format string) bool {
	for _, f := range kvAvailableFormats {
		if f == format {
			return true
		}
	}

	return false
}

// Convert KV Pairs struct to JSON and print it on stdout
func printKVPairsJSON(inp interface{}) {
	bytes, err := json.MarshalIndent(inp, "", "  ")
	if err != nil {
		errLog.Fatalf("error marshelling output: %v", err)
	}

	sysLog.Println(string(bytes[:]))
}

// Parse config file to a map
func configToMap(cType string, r io.Reader) (map[string]interface{}, error) {
	viper.SetConfigType(cType)
	err := viper.ReadConfig(r)
	if err != nil {
		return nil, err
	}

	return viper.AllSettings(), nil
}

// Recursively traverse map and insert KV Pair to output if it can't be further traversed.
func mapToKVPairs(ckv *[]consulKVPair, prefix string, inp map[string]interface{}) {
	for k, v := range inp {
		var newPrefix string
		// If prefix is empty then don't append "/" else form a new prefix with current key.
		if prefix == "" {
			newPrefix = k
		} else {
			newPrefix = prefix + "/" + k
		}

		// Check if value is a map. If map then traverse further else write to output as a KVPair.
		vKind := reflect.TypeOf(v).Kind()
		if vKind == reflect.Map {
			m, ok := v.(map[string]interface{})
			if !ok {
				errLog.Fatalf("not ok: %v - %v\n", k, v)
			}

			mapToKVPairs(ckv, newPrefix, m)
		} else {
			// If its not  string then encode it using JSON
			// CAVEAT: TOML supports array of maps but consul KV doesn't support this so it will be JSON marshalled.
			var val string
			if vKind == reflect.String {
				val = v.(string)
			} else {
				vJSON, err := json.Marshal(v)
				if err != nil {
					errLog.Fatalf("error while marshalling value: %v err: %v", v, err)
				}
				val = string(vJSON)
			}

			*ckv = append(*ckv, consulKVPair{
				Flags: 0,
				Key:   newPrefix,
				Value: val,
			})
		}
	}
}
