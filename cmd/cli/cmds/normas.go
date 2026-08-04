package cmds

import (
	"fmt"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/spf13/cobra"
)

type normasCmdFlags struct {
	tipo      string
	numero    string
	titulo    string
	assunto   string
	situacao  string
	dataDe    string
	dataAte   string
	page      int
	pageSize  int
	outputFmt string
}

func NewNormasCmd() *cobra.Command {
	flags := &normasCmdFlags{}

	cmd := &cobra.Command{
		Use:   "normas",
		Short: "Query norms from the database",
		Long:  "Query and list Banco Central do Brasil norms stored in the database with optional filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			return runNormas(app, flags)
		},
	}

	cmd.Flags().StringVarP(&flags.tipo, "tipo", "t", "", "Filter by norm type")
	cmd.Flags().StringVarP(&flags.numero, "numero", "n", "", "Filter by norm number")
	cmd.Flags().StringVarP(&flags.titulo, "titulo", "T", "", "Filter by title")
	cmd.Flags().StringVarP(&flags.assunto, "assunto", "a", "", "Filter by subject")
	cmd.Flags().StringVarP(&flags.situacao, "situacao", "s", "", "Filter by situation")
	cmd.Flags().StringVarP(&flags.dataDe, "data-de", "", "", "Filter by publication date from (YYYY-MM-DD)")
	cmd.Flags().StringVarP(&flags.dataAte, "data-ate", "", "", "Filter by publication date until (YYYY-MM-DD)")
	cmd.Flags().IntVarP(&flags.page, "page", "p", 1, "Page number")
	cmd.Flags().IntVarP(&flags.pageSize, "page-size", "P", 50, "Items per page")
	cmd.Flags().StringVarP(&flags.outputFmt, "output", "o", "text", "Output format (text, json)")

	return cmd
}

func runNormas(app *App, flags *normasCmdFlags) error {
	search := &models.NormaSearch{
		Page:     flags.page,
		PageSize: flags.pageSize,
	}

	if flags.tipo != "" {
		t := models.TipoNorma(flags.tipo)
		search.Tipo = &t
	}

	if flags.numero != "" {
		search.Numero = &flags.numero
	}

	if flags.titulo != "" {
		search.Titulo = &flags.titulo
	}

	if flags.assunto != "" {
		search.Assunto = &flags.assunto
	}

	if flags.situacao != "" {
		search.Situacao = &flags.situacao
	}

	if flags.dataDe != "" {
		if date, err := time.Parse("2006-01-02", flags.dataDe); err == nil {
			search.DataDe = &date
		}
	}

	if flags.dataAte != "" {
		if date, err := time.Parse("2006-01-02", flags.dataAte); err == nil {
			search.DataAte = &date
		}
	}

	normas, total, err := app.Storage.ListNormas(search)
	if err != nil {
		return fmt.Errorf("failed to list normas: %w", err)
	}

	totalPages := total / search.PageSize
	if total%search.PageSize > 0 {
		totalPages++
	}

	if outputFmt == "json" {
		response := models.NormaResponse{
			Normas:     normas,
			Total:      total,
			Page:       search.Page,
			PageSize:   search.PageSize,
			TotalPages: totalPages,
		}
		return outputJSON(response)
	}

	printNormasTable(normas, total, search.Page, totalPages)
	return nil
}

func printNormasTable(normas []models.Norma, total, page, totalPages int) {
	fmt.Printf("\n%-5s %-15s %-12s %-50s %-12s %-10s\n", "ID", "Numero", "Tipo", "Titulo", "Situacao", "Publicacao")
	fmt.Println("--------------------------------------------------------------------------------------------")

	for _, n := range normas {
		titulo := n.Titulo
		if len(titulo) > 47 {
			titulo = titulo[:47] + "..."
		}
		pubDate := n.DataPublicacao
		if len(pubDate) > 10 {
			pubDate = pubDate[:10]
		}
		fmt.Printf("%-5d %-15s %-12s %-50s %-12s %-10s\n",
			n.ID, n.Numero, n.Tipo, titulo, n.Situacao, pubDate)
	}

	fmt.Printf("\nTotal: %d | Page %d of %d\n", total, page, totalPages)
}