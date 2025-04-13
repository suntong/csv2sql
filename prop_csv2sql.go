package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// CSVToMySQLConverter handles the conversion process
type CSVToMySQLConverter struct {
	OptsT
	NullString  string
	ForceTypes  map[string]string // column name -> MySQL type
	SkipColumns map[string]bool   // columns to skip
}

var (
	sanitizeRegex = regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	leadingRegex  = regexp.MustCompile(`^[^a-zA-Z_]`)
)

// NewCSVToMySQLConverter creates a new converter instance
func NewCSVToMySQLConverter(optsT OptsT) *CSVToMySQLConverter {
	return &CSVToMySQLConverter{
		OptsT:       optsT,
		NullString:  "NULL",
		ForceTypes:  make(map[string]string),
		SkipColumns: make(map[string]bool),
	}
}

// Convert processes the CSV file and generates MySQL statements
func (c *CSVToMySQLConverter) Convert() (string, string, error) {
	file, err := os.Open(c.InputFile)
	if err != nil {
		return "", "", fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = rune(strings.TrimSpace(c.Delimiter)[0])
	reader.TrimLeadingSpace = true

	headers, err := c.readHeaders(reader)
	if err != nil {
		return "", "", fmt.Errorf("error reading headers: %w", err)
	}

	columnTypes, err := c.determineColumnTypes(reader, headers)
	if err != nil {
		return "", "", fmt.Errorf("error determining column types: %w", err)
	}

	// Generate CREATE TABLE statement
	createTable := c.generateCreateTable(headers, columnTypes)

	// Generate INSERT statements
	inserts, err := c.generateInsertStatements(file, headers, columnTypes)
	if err != nil {
		return "", "", fmt.Errorf("error generating insert statements: %w", err)
	}

	return createTable, inserts, nil
}

func (c *CSVToMySQLConverter) readHeaders(reader *csv.Reader) ([]string, error) {
	if !c.NoHeader {
		rawHeaders, err := reader.Read()
		if err != nil {
			return nil, fmt.Errorf("error reading header: %w", err)
		}

		headers := make([]string, len(rawHeaders))
		for i, h := range rawHeaders {
			headers[i] = c.sanitizeColumnName(h)
			if headers[i] == "" {
				headers[i] = fmt.Sprintf("column_%d", i+1)
			}
		}
		return headers, nil
	}

	firstRow, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("error reading first row: %w", err)
	}

	headers := make([]string, len(firstRow))
	for i := range firstRow {
		headers[i] = fmt.Sprintf("column_%d", i+1)
	}

	file, err := os.Open(c.InputFile)
	if err != nil {
		return nil, fmt.Errorf("error reopening file: %w", err)
	}
	defer file.Close()

	reader = csv.NewReader(file)
	reader.Comma = rune(strings.TrimSpace(c.Delimiter)[0])
	reader.TrimLeadingSpace = true

	return headers, nil
}

func (c *CSVToMySQLConverter) sanitizeColumnName(name string) string {
	// Clean special characters
	name = sanitizeRegex.ReplaceAllString(strings.TrimSpace(name), "_")
	name = strings.Trim(name, "_")

	// Ensure valid starting character
	if leadingRegex.MatchString(name) {
		name = "_" + name
	}

	// Ensure lowercase
	return strings.ToLower(name)
}

func (c *CSVToMySQLConverter) determineColumnTypes(reader *csv.Reader, headers []string) ([]string, error) {
	columnTypes := make([]string, len(headers))
	for i := range headers {
		if forcedType, ok := c.ForceTypes[headers[i]]; ok {
			columnTypes[i] = forcedType
		} else if c.SkipColumns[headers[i]] {
			columnTypes[i] = "SKIP"
		} else {
			columnTypes[i] = "TEXT"
		}
	}

	// If we have forced types for all columns, skip analysis
	allForced := true
	for _, t := range columnTypes {
		if !strings.HasPrefix(t, "VARCHAR") && t != "SKIP" {
			allForced = false
			break
		}
	}
	if allForced {
		return columnTypes, nil
	}

	// Sample up to 1000 rows to determine types
	sampleCount := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"[determineColumnTypes] Warning: error reading record: %v\n"+
					"\tSkipping the record of: %#v\n", err, record)
			continue
		}

		if len(record) != len(headers) {
			log.Printf("Skipping row with %d columns (expected %d)", len(record), len(headers))
			continue
		}

		for i, value := range record {
			if columnTypes[i] == "SKIP" {
				continue
			}

			// Skip if type is forced
			if _, ok := c.ForceTypes[headers[i]]; ok {
				continue
			}

			value = strings.TrimSpace(value)
			if value == "" || strings.EqualFold(value, c.NullString) {
				continue
			}

			if _, ok := c.ForceTypes[headers[i]]; !ok {
				columnTypes[i] = c.refineType(columnTypes[i], value)
			}
		}

		sampleCount++
		if sampleCount >= c.MaxSampleSize {
			break
		}
	}

	return columnTypes, nil
}

func (c *CSVToMySQLConverter) refineType(currentType, value string) string {
	if isInteger(value) {
		return "BIGINT"
	}
	if isDecimal(value) {
		return "DECIMAL(20,6)"
	}
	if isDate(value) {
		if len(value) > 10 {
			return "DATETIME"
		}
		return "DATE"
	}

	length := len(value)
	if length > c.TextThreshold {
		return "TEXT"
	}
	if length > c.VarcharLength {
		return fmt.Sprintf("VARCHAR(%d)", ((length/50)+1)*50)
	}
	return fmt.Sprintf("VARCHAR(%d)", c.VarcharLength)
}

// generateCreateTable generates the MySQL CREATE TABLE statement
func (c *CSVToMySQLConverter) generateCreateTable(headers []string, columnTypes []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("-- DROP TABLE %s;\n", c.TableName))
	sb.WriteString(fmt.Sprintf("CREATE TABLE `%s` (\n", c.TableName))

	columns := make([]string, 0, len(headers))
	for i, header := range headers {
		if columnTypes[i] == "SKIP" {
			continue
		}
		columns = append(columns, fmt.Sprintf("  `%s` %s", header, columnTypes[i]))
	}

	// Add primary key if specified
	if len(c.PrimaryKeys) > 0 {
		pkColumns := make([]string, 0, len(c.PrimaryKeys))
		for _, pk := range c.PrimaryKeys {
			pkColumns = append(pkColumns, fmt.Sprintf("`%s`", pk))
		}
		columns = append(columns, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(pkColumns, ", ")))
	}

	sb.WriteString(strings.Join(columns, ",\n"))
	sb.WriteString("\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;")

	return sb.String()
}

// generateInsertStatements generates MySQL INSERT statements
func (c *CSVToMySQLConverter) generateInsertStatements(file *os.File, headers []string, columnTypes []string) (string, error) {
	// Reset file reader
	file.Seek(0, 0)
	reader := csv.NewReader(file)
	reader.Comma = rune(strings.TrimSpace(c.Delimiter)[0])

	if !c.NoHeader {
		reader.Read()
	}

	var sb strings.Builder
	var batchRows []string
	rowCount := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"[generateInsertStatements] Warning: error reading record: %v\n"+
					"\tSkipping the record of: %#v\n", err, record)
			continue
		}

		if len(record) != len(headers) {
			fmt.Printf("column count mismatch: expected %d, got %d", len(headers), len(record))
			continue
		}

		// Skip empty rows
		col := strings.TrimSpace(record[0])
		isNull := col == "" ||
			strings.EqualFold(col, c.NullString)
		if len(record) >= 2 {
			col = strings.TrimSpace(record[1])
			isNull = isNull && (col == "" ||
				strings.EqualFold(col, c.NullString))
		}
		if isNull {
			continue
		}

		// Prepare values
		values := make([]string, 0, len(headers))
		for i, value := range record {
			if columnTypes[i] == "SKIP" {
				continue
			}

			value = strings.TrimSpace(value)
			if value == "" || strings.EqualFold(value, c.NullString) {
				values = append(values, "NULL")
				continue
			}

			// Escape special characters
			escaped := strings.ReplaceAll(value, "'", "''")
			escaped = strings.ReplaceAll(escaped, "\\", "\\\\")

			// Add quotes unless it's a number or NULL
			if columnTypes[i] == "INT" || columnTypes[i] == "DECIMAL(20,6)" {
				// Try to parse as number to validate
				if _, err := strconv.ParseFloat(value, 64); err == nil {
					values = append(values, escaped)
					continue
				}
			}
			values = append(values, fmt.Sprintf("'%s'", escaped))
		}

		if !c.NoBatchInsert {
			batchRows = append(batchRows, fmt.Sprintf("(%s)", strings.Join(values, ", ")))
			if len(batchRows) >= c.BatchSize {
				sb.WriteString(c.formatBatchInsert(headers, columnTypes, batchRows))
				batchRows = batchRows[:0] // Clear batch
			}
		} else {
			sb.WriteString(fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s);\n",
				c.TableName,
				c.formatInsertColumns(headers, columnTypes),
				strings.Join(values, ", ")))
		}

		rowCount++
	}

	// Write any remaining batched rows
	if len(batchRows) > 0 {
		sb.WriteString(c.formatBatchInsert(headers, columnTypes, batchRows))
	}

	return sb.String(), nil
}

// formatInsertColumns formats the column list for INSERT statements
func (c *CSVToMySQLConverter) formatInsertColumns(headers []string, columnTypes []string) string {
	var cols []string
	for i, h := range headers {
		if columnTypes[i] != "SKIP" {
			cols = append(cols, fmt.Sprintf("`%s`", h))
		}
	}
	return strings.Join(cols, ", ")
}

// formatBatchInsert formats a batch INSERT statement
func (c *CSVToMySQLConverter) formatBatchInsert(headers []string, columnTypes []string, rows []string) string {
	return fmt.Sprintf("INSERT INTO `%s` (%s) VALUES\n%s;\n",
		c.TableName,
		c.formatInsertColumns(headers, columnTypes),
		strings.Join(rows, ",\n"))
}

// Helper functions from second implementation
func isInteger(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func isDecimal(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func isDate(s string) bool {
	// Simple date patterns
	patterns := []string{
		`^\d{4}-\d{2}-\d{2}$`,                     // YYYY-MM-DD
		`^\d{2}/\d{2}/\d{4}$`,                     // MM/DD/YYYY
		`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`,   // YYYY-MM-DD HH:MM:SS
		`^\d{2}/\d{2}/\d{4} \d{2}:\d{2}:\d{2}$`,   // MM/DD/YYYY HH:MM:SS
		`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z?$`, // ISO8601
	}

	for _, pattern := range patterns {
		matched, _ := regexp.MatchString(pattern, s)
		if matched {
			return true
		}
	}
	return false
}

// Escaping function from second implementation
func escapeSQLValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "NULL") {
		return "NULL"
	}

	escaped := strings.ReplaceAll(trimmed, "'", "''")
	escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
	return "'" + escaped + "'"
}

/*
func main() {
	converter := NewCSVToMySQLConverter()
	converter.InputFile = "data.csv"
	converter.TableName = "sales_data"
	converter.PrimaryKeys = []string{"order_id"}
	converter.ForceTypes = map[string]string{
		"order_id": "INT AUTO_INCREMENT",
		"price":    "DECIMAL(10,2)",
	}
	converter.SkipColumns = map[string]bool{"internal_code": true}

	createStmt, insertStmts, err := converter.Convert()
	if err != nil {
		log.Fatalf("Error converting CSV to SQL: %v", err)
	}

	fmt.Println("-- CREATE TABLE STATEMENT --")
	fmt.Println(createStmt)
	fmt.Println("\n-- INSERT STATEMENTS --")
	fmt.Println(insertStmts)
}
*/
