// csv2sql - CSV to SQL DDL+DML
//
// CSV to SQL insert statements

package csv2sql

////////////////////////////////////////////////////////////////////////////
// Program: csv2sql
// Purpose: CSV to SQL DDL+DML
// Authors: Tong Sun (c) 2025-2025, All rights reserved
////////////////////////////////////////////////////////////////////////////

import (
//  	"fmt"
//  	"os"

// "github.com/go-easygen/go-flags"
)

// Template for main starts here

//  // for `go generate -x`
//  //go:generate sh csv2sql_cliGen.sh

//////////////////////////////////////////////////////////////////////////
// Constant and data type/structure definitions

////////////////////////////////////////////////////////////////////////////
// Global variables definitions

//  var (
//          progname  = "csv2sql"
//          version   = "0.1.0"
//          date = "2025-04-13"

//  	// Opts store all the configurable options
//  	Opts OptsT
//  )
//
//  var gfParser = flags.NewParser(&Opts, flags.Default)

////////////////////////////////////////////////////////////////////////////
// Function definitions

//==========================================================================
// Function main
//  func main() {
//  	Opts.Version = showVersion
//  	Opts.Verbflg = func() {
//  		Opts.Verbose++
//  	}
//
//  	if _, err := gfParser.Parse(); err != nil {
//  		fmt.Println()
//  		gfParser.WriteHelp(os.Stdout)
//  		os.Exit(1)
//  	}
//  	fmt.Println()
//  	//DoCsv2sql()
//  }
//
//  //==========================================================================
//  // support functions
//
//  func showVersion() {
//   	fmt.Fprintf(os.Stderr, "csv2sql - CSV to SQL DDL+DML, version %s\n", version)
//  	fmt.Fprintf(os.Stderr, "Built on %s\n", date)
//   	fmt.Fprintf(os.Stderr, "Copyright (C) 2025-2025, Tong Sun\n\n")
//  	fmt.Fprintf(os.Stderr, "CSV to SQL insert statements\n")
//  	os.Exit(0)
//  }
// Template for main ends here

// DoCsv2sql implements the business logic of command `csv2sql`
//  func DoCsv2sql() error {
//  	return nil
//  }

// Template for type define starts here

// The OptsT type defines all the configurable options from cli.
type OptsT struct {
	InputFile     string   `short:"i" env:"CSV2SQL_INPUTFILE" description:"Input .csv file" required:"true"`
	TableName     string   `short:"t" env:"CSV2SQL_TABLENAME" description:"Table name to hold csv data" required:"true"`
	PrimaryKeys   []string `short:"k" env:"CSV2SQL_PRIMARYKEYS" description:"Primary keys of the table"`
	Delimiter     string   `short:"d" env:"CSV2SQL_DELIMITER" description:"Delimiter char of csv data" default:","`
	NoHeader      bool     `short:"H" env:"CSV2SQL_NOHEADER" description:"Not having csv header"`
	NoBatchInsert bool     `short:"B" long:"bi" env:"CSV2SQL_NOBATCHINSERT" description:"No batch insert"`
	BatchSize     int      `long:"bs" env:"CSV2SQL_BATCHSIZE" description:"BatchSize" default:"100"`
	VarcharLength int      `long:"vl" env:"CSV2SQL_VARCHARLENGTH" description:"Varchar length" default:"255"`
	TextThreshold int      `long:"tt" env:"CSV2SQL_TEXTTHRESHOLD" description:"Text length threshold" default:"100"`
	MaxSampleSize int      `long:"mss" env:"CSV2SQL_MAXSAMPLESIZE" description:"Max sample size to determine column types" default:"1000"`
	Verbflg       func()   `short:"v" long:"verbose" description:"Verbose mode (Multiple -v options increase the verbosity)"`
	Verbose       int
	Version       func() `short:"V" long:"version" description:"Show program version and exit"`
}

// Template for type define ends here
