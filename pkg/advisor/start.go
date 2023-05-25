package advisor

import (
	"flag"
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

func init() {
	_ = flag.Set("logtostderr", "true")
	// hack to make flag.Parsed return true such that glog is happy
	// about the flags having been parsed
	_ = flag.CommandLine.Parse([]string{})
}

// Execute will execute basically the whole application
func Execute() {
	options := &Options{}
	_ = flag.Lookup("logtostderr").Value.Set("true")
	glog.Flush()
	rootCmd := &cobra.Command{
		Use:   "resource-advisor",
		Short: "Kubernetes resource-advisor",
		Long:  "Kubernetes resource-advisor",
		Run: func(cmd *cobra.Command, args []string) {
			_, err := Run(options)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n%v\n", err)
				os.Exit(1)
				return
			}
		},
	}

	rootCmd.Flags().StringVarP(&options.Namespaces, "namespaces", "n", "", "Comma separated namespaces to be scanned")
	rootCmd.Flags().StringVarP(&options.NamespaceSelector, "namespace-selector", "l", "", "Namespace selector")
	rootCmd.Flags().StringVarP(&options.Quantile, "quantile", "q", "0.95", "Quantile to be used")
	rootCmd.Flags().StringVarP(&options.LimitMargin, "limit-margin", "m", "1.2", "Limit margin")
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
