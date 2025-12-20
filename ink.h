#ifndef INK_H
#define INK_H

#include <errno.h>
#include <fcntl.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define KEY_SIZE 32
#define VALUE_SIZE 256
#define MAX_ENTRIES 100
#define INDEX_CAPACITY 1024
#define RECORD_MAGIC 0xCAFEBABE

extern const char db_file_name[];

typedef struct {
  char key[KEY_SIZE];
  off_t offset;
  int used; // 0 = empty, 1 = occupied, -1 = tomstone
} IndexSlot;

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

void db_insert(const char *filename, const char *key, const char *value);
void db_get_at(const char *filename, off_t offset, char *out_value);
void input_loop();
void build_index(const char *filename);
void compact();

#endif
