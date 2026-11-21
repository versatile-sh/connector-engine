package sqlite

// SetWriterSchema makes the schema of kind's snapshots schema for one test and returns what undoes it.
func SetWriterSchema(kind string, schema int) (undo func()) {
	was := writerSchemas[kind]
	writerSchemas[kind] = func() int { return schema }
	return func() { writerSchemas[kind] = was }
}
