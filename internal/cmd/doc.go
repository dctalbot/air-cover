package cmd

import (
	"fmt"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/docgen"
	"github.com/spf13/cobra"

	"air-cover/internal/api"
)

var docCmd = &cobra.Command{
	Use:   "doc",
	Short: "Generate route documentation",
	Long:  `Generate JSON documentation for the registered routes.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Minimal setup for docgen
		// We don't need real dependencies for docgen as it only inspects the router structure
		authHandler := api.NewAuthHandler(nil, nil)
		apiServer := api.NewServer(nil, authHandler, nil)

		r := newRouter(apiServer, authHandler)
		fmt.Println(docgen.JSONRoutesDoc(r.(*chi.Mux))) // nolint:forbidigo
	},
}

func init() {
	rootCmd.AddCommand(docCmd)
}
