package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// Config holds the configuration for the conversion
type Config struct {
	InputFile      string
	TableName      string
	Delimiter      rune
	HasHeader      bool
	VarcharLength  int
	TextThreshold  int
	BatchInsert    bool
	BatchSize      int
	NullString     string
	ForceTypes     map[string]string // column name -> MySQL type
	SkipColumns    map[string]bool   // columns to skip
	PrimaryKeys    []string          // columns to set as primary keys
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		Delimiter:     ',',
		HasHeader:     true,
		VarcharLength: 255,
		TextThreshold: 500,
		BatchInsert:   true,
		BatchSize:     100,
		NullString:    "NULL",
		ForceTypes:    make(map[string]string),
		SkipColumns:   make(map[string]bool),
	}
}

// CSVToMySQLConverter handles the conversion process
type CSVToMySQLConverter struct {
	config Config
}

// NewCSVToMySQLConverter creates a new converter instance
func NewCSVToMySQLConverter(config Config) *CSVToMySQLConverter {
	return &CSVToMySQLConverter{config: config}
}

// Convert processes the CSV file and generates MySQL statements
func (c *CSVToMySQLConverter) Convert() (string, string, error) {
	file, err := os.Open(c.config.InputFile)
	if err != nil {
		return "", "", fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = c.config.Delimiter

	// Read header
	var headers []string
	if c.config.HasHeader {
		headers, err = reader.Read()
		if err != nil {
			return "", "", fmt.Errorf("error reading header: %w", err)
		}
		// Clean up headers
		for i, h := range headers {
			headers[i] = c.sanitizeColumnName(h)
		}
	}

	// Read first row to determine column count if no header
	var firstRow []string
	if !c.config.HasHeader {
		firstRow, err = reader.Read()
		if err != nil {
			return "", "", fmt.Errorf("error reading first row: %w", err)
		}
		headers = make([]string, len(firstRow))
		for i := range firstRow {
			headers[i] = fmt.Sprintf("column_%d", i+1)
		}
		// Reset reader to beginning
		file.Seek(0, 0)
		reader = csv.NewReader(file)
		reader.Comma = c.config.Delimiter
	}

	// Process all rows to determine column types
	columnTypes, err := c.determineColumnTypes(reader, headers, firstRow)
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

// sanitizeColumnName cleans up column names for SQL
func (c *CSVToMySQLConverter) sanitizeColumnName(name string) string {
	// Trim whitespace
	name = strings.TrimSpace(name)
	
	// Remove special characters
	var sb strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			sb.WriteRune(r)
		} else if unicode.IsSpace(r) {
			sb.WriteRune('_')
		}
	}
	
	result := sb.String()
	if result == "" {
		return "column"
	}
	
	// Ensure it doesn't start with a number
	if unicode.IsDigit(rune(result[0])) {
		result = "col_" + result
	}
	
	return strings.ToLower(result)
}

// determineColumnTypes analyzes the CSV data to determine appropriate MySQL types
func (c *CSVToMySQLConverter) determineColumnTypes(reader *csv.Reader, headers []string, firstRow []string) ([]string, error) {
	// Reset reader if we've already read the first row
	if firstRow != nil {
		reader = csv.NewReader(io.MultiReader(
			strings.NewReader(strings.Join(firstRow, string(c.config.Delimiter))+"\n"),
			os.Stdin,
		))
		reader.Comma = c.config.Delimiter
	}

	columnTypes := make([]string, len(headers))
	for i := range columnTypes {
		if forcedType, ok := c.config.ForceTypes[headers[i]]; ok {
			columnTypes[i] = forcedType
		} else if c.config.SkipColumns[headers[i]] {
			columnTypes[i] = "SKIP"
		} else {
			columnTypes[i] = "VARCHAR(255)" // Default, will be refined
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
	sampleSize := 0
	maxSampleSize := 1000

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading record: %w", err)
		}

		if len(record) != len(headers) {
			return nil, fmt.Errorf("column count mismatch: expected %d, got %d", len(headers), len(record))
		}

		for i, value := range record {
			if columnTypes[i] == "SKIP" {
				continue
			}

			// Skip if type is forced
			if _, ok := c.config.ForceTypes[headers[i]]; ok {
				continue
			}

			value = strings.TrimSpace(value)
			if value == "" || strings.EqualFold(value, c.config.NullString) {
				continue
			}

			currentType := columnTypes[i]

			// Check for integer
			if _, err := strconv.ParseInt(value, 10, 64); err == nil {
				if currentType == "VARCHAR(255)" {
					columnTypes[i] = "INT"
				}
				continue
			}

			// Check for decimal
			if _, err := strconv.ParseFloat(value, 64); err == nil {
				if currentType == "VARCHAR(255)" || currentType == "INT" {
					columnTypes[i] = "DECIMAL(20,6)"
				}
				continue
			}

			// Check for date/datetime
			if isDate(value) {
				if len(value) > 10 {
					columnTypes[i] = "DATETIME"
				} else {
					columnTypes[i] = "DATE"
				}
				continue
			}

			// It's some kind of string
			length := len(value)
			if currentType == "VARCHAR(255)" {
				if length > c.config.TextThreshold {
					columnTypes[i] = "TEXT"
				} else if length > c.config.VarcharLength {
					newLength := ((length / 50) + 1) * 50 // Round up to nearest 50
					if newLength > 65535 {
						columnTypes[i] = "TEXT"
					} else {
						columnTypes[i] = fmt.Sprintf("VARCHAR(%d)", newLength)
					}
				}
			} else if strings.HasPrefix(currentType, "VARCHAR(") {
				if length > c.extractVarcharLength(currentType) {
					if length > c.config.TextThreshold {
						columnTypes[i] = "TEXT"
					} else {
						newLength := ((length / 50) + 1) * 50 // Round up to nearest 50
						if newLength > 65535 {
							columnTypes[i] = "TEXT"
						} else {
							columnTypes[i] = fmt.Sprintf("VARCHAR(%d)", newLength)
						}
					}
				}
			}
		}

		sampleSize++
		if sampleSize >= maxSampleSize {
			break
		}
	}

	return columnTypes, nil
}

// extractVarcharLength gets the length from a VARCHAR(n) definition
func (c *CSVToMySQLConverter) extractVarcharLength(typeDef string) int {
	start := strings.Index(typeDef, "(")
	end := strings.Index(typeDef, ")")
	if start == -1 || end == -1 {
		return c.config.VarcharLength
	}
	length, err := strconv.Atoi(typeDef[start+1 : end])
	if err != nil {
		return c.config.VarcharLength
	}
	return length
}

// generateCreateTable generates the MySQL CREATE TABLE statement
func (c *CSVToMySQLConverter) generateCreateTable(headers []string, columnTypes []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE `%s` (\n", c.config.TableName))

	columns := make([]string, 0, len(headers))
	for i, header := range headers {
		if columnTypes[i] == "SKIP" {
			continue
		}
		columns = append(columns, fmt.Sprintf("  `%s` %s", header, columnTypes[i]))
	}

	// Add primary key if specified
	if len(c.config.PrimaryKeys) > 0 {
		var pkColumns []string
		for _, pk := range c.config.PrimaryKeys {
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
	reader.Comma = c.config.Delimiter

	// Skip header if present
	if c.config.HasHeader {
		_, err := reader.Read()
		if err != nil {
			return "", fmt.Errorf("error skipping header: %w", err)
		}
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
			return "", fmt.Errorf("error reading record: %w", err)
		}

		if len(record) != len(headers) {
			return "", fmt.Errorf("column count mismatch: expected %d, got %d", len(headers), len(record))
		}

		// Prepare values
		values := make([]string, 0, len(headers))
		for i, value := range record {
			if columnTypes[i] == "SKIP" {
				continue
			}

			value = strings.TrimSpace(value)
			if value == "" || strings.EqualFold(value, c.config.NullString) {
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

		if c.config.BatchInsert {
			batchRows = append(batchRows, fmt.Sprintf("(%s)", strings.Join(values, ", ")))
			if len(batchRows) >= c.config.BatchSize {
				sb.WriteString(c.formatBatchInsert(headers, columnTypes, batchRows))
				batchRows = batchRows[:0] // Clear batch
			}
		} else {
			sb.WriteString(fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s);\n",
				c.config.TableName,
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
		c.config.TableName,
		c.formatInsertColumns(headers, columnTypes),
		strings.Join(rows, ",\n"))
}

// isDate checks if a string looks like a date
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

func main() {
	// Example usage
	config := DefaultConfig()
	config.InputFile = "input.csv"
	config.TableName = "my_table"
	config.PrimaryKeys = []string{"id"}
	config.ForceTypes = map[string]string{
		"id":          "INT AUTO_INCREMENT",
		"price":       "DECIMAL(10,2)",
		"description": "TEXT",
	}
	config.SkipColumns = map[string]bool{
		"internal_code": true,
	}

	converter := NewCSVToMySQLConverter(config)
	createTable, inserts, err := converter.Convert()
	if err != nil {
		log.Fatalf("Error converting CSV to MySQL: %v", err)
	}

	fmt.Println("-- CREATE TABLE STATEMENT:")
	fmt.Println(createTable)
	fmt.Println("\n-- INSERT STATEMENTS:")
	fmt.Println(inserts)
}