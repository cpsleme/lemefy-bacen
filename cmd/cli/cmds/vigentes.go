package cmds

import (
	"fmt"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/spf13/cobra"
)

type vigentesCmdFlags struct {
	numero   string
	titulo   string
	assunto  string
	tipo     string
	dataDe   string
	dataAte  string
	page     int
	pageSize int
}

func NewVigentesCmd() *cobra.Command {
	flags := &vigentesCmdFlags{}

	cmd := &cobra.Command{
		Use:   "vigentes",
		Short: "List only active norms",
		Long:  "List Banco Central do Brasil norms that are currently active (situacao = Vigente)",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			return runVigentes(app, flags)
		},
	}

	cmd.Flags().StringVarP(&flags.tipo, "tipo", "t", "", "Filter by norm type")
	cmd.Flags().StringVarP(&flags.numero, "numero", "n", "", "Filter by norm number")
	cmd.Flags().StringVarP(&flags.titulo, "titulo", "T", "", "Filter by title")
	cmd.Flags().StringVarP(&flags.assunto, "assunto", "a", "", "Filter by subject")
	cmd.Flags().StringVarP(&flags.dataDe, "data-de", "", "", "Filter by publication date from (YYYY-MM-DD)")
	cmd.Flags().StringVarP(&flags.dataAte, "data-ate", "", "", "Filter by publication date until (YYYY-MM-DD)")
	cmd.Flags().IntVarP(&flags.page, "page", "p", 1, "Page number")
	cmd.Flags().IntVarP(&flags.pageSize, "page-size", "P", 50, "Items per page")

	return cmd
}

func runVigentes(app *App, flags *vigentesCmdFlags) error {
	situacao := "Vigente"
	search := &models.NormaSearch{
		Page:     flags.page,
		PageSize: flags.pageSize,
		Situacao: &situacao,
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
