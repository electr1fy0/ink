#ifndef INK_H
#define INK_H

#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <stdint.h>

#define KEY_SIZE 32
#define VALUE_SIZE 256
#define MAX_ENTRIES 100
#define RECORD_MAGIC 0xCAFEBABE
extern const char db_file_name[];

typedef struct {
  char key[KEY_SIZE];
  char value[VALUE_SIZE];
} Record;

typedef struct {
  uint32_t magic;
  uint8_t deleted;
  Record record;
} DiskRecord;

typedef struct {
  char key[KEY_SIZE];
  off_t offset;
} IndexEntry;

extern IndexEntry index_entries[MAX_ENTRIES];
extern int index_entry_len;

IndexEntry db_insert(const char *filename, const char *key, const char *value);
void db_get_at(const char *filename, off_t offset, char *out_value);
int return_index(const IndexEntry *indexes, char key[], int n);
void input_loop();
void build_index(const char *filename);
void compact();

#endif
