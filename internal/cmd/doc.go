package cmd

import (
	"fmt"

	"air-cover/internal/adapters/inbound/http/api"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/docgen"
	"github.com/spf13/cobra"
)

var docCmd = &cobra.Command{
	Use:   "doc",
	Short: "Generate route documentation",
	Long:  `Generate JSON documentation for the registered routes.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Minimal setup for docgen
		// We don't need real dependencies for docgen as it only inspects the router structure
		authHandler := api.NewAuthHandler(nil)
		apiServer := api.NewServer(authHandler, nil, nil)

		r := api.NewRouter(apiServer, authHandler)
		fmt.Println(docgen.JSONRoutesDoc(r.(*chi.Mux))) // nolint:forbidigo
	},
}

func init() {
	rootCmd.AddCommand(docCmd)
}
