// csv2sql - CSV to SQL DDL+DML
//
// CSV to SQL insert statements

package main

////////////////////////////////////////////////////////////////////////////
// Program: csv2sql
// Purpose: CSV to SQL DDL+DML
// Authors: Tong Sun (c) 2025-2025, All rights reserved
////////////////////////////////////////////////////////////////////////////

import (
	"fmt"
	"log"
	"os"

	"github.com/go-easygen/go-flags"

	"github.com/suntong/csv2sql"
)

//////////////////////////////////////////////////////////////////////////
// Constant and data type/structure definitions

////////////////////////////////////////////////////////////////////////////
// Global variables definitions

var (
	progname = "csv2sql"
	version  = "0.1.0"
	date     = "2025-04-12"

	// Opts store all the configurable options
	Opts csv2sql.OptsT
)

var gfParser = flags.NewParser(&Opts, flags.Default)

////////////////////////////////////////////////////////////////////////////
// Function definitions

// ==========================================================================
// Function main
func main() {
	Opts.Version = showVersion
	Opts.Verbflg = func() {
		Opts.Verbose++
	}
	//
	if _, err := gfParser.Parse(); err != nil {
		fmt.Println()
		gfParser.WriteHelp(os.Stdout)
		os.Exit(1)
	}
	fmt.Println()
	DoCsv2sql()
}

// ==========================================================================
// support functions
func showVersion() {
	fmt.Fprintf(os.Stderr, "csv2sql - CSV to SQL DDL+DML, version %s\n", version)
	fmt.Fprintf(os.Stderr, "Built on %s\n", date)
	fmt.Fprintf(os.Stderr, "Copyright (C) 2025-2025, Tong Sun\n\n")
	fmt.Fprintf(os.Stderr, "CSV to SQL insert statements\n")
	os.Exit(0)
}

// DoCsv2sql implements the business logic of command `csv2sql`
func DoCsv2sql() error {
	converter := csv2sql.NewCSVToMySQLConverter(Opts)
	// converter.ForceTypes = map[string]string{
	// 	"order_id": "INT AUTO_INCREMENT",
	// 	"price":    "DECIMAL(10,2)",
	// }
	// converter.SkipColumns = map[string]bool{"internal_code": true}

	createStmt, insertStmts, err := converter.Convert()
	if err != nil {
		log.Fatalf("Error converting CSV to SQL: %v", err)
	}

	fmt.Println("-- CREATE TABLE STATEMENT --")
	fmt.Println(createStmt)
	fmt.Println("\n-- INSERT STATEMENTS --")
	fmt.Println(insertStmts)
	return nil
}
