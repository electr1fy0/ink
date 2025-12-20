#include "ink.h"


IndexSlot index_table[INDEX_CAPACITY];

static uint32_t hash_key(const char *key) {
  uint32_t h = 2166136261u; // FNV-1 32 bit hash offset
  for (int i = 0; i < KEY_SIZE && key[i]; ++i) {
    h ^= (unsigned char)key[i];
    h *= 16777619u; // a prime number used in 32-bit moduleo arithmetic
  }

  return h;
}

/*
 * Returns the on-disk offset for key.
 *  -1 if not found.
 */
off_t index_lookup(const char *key) {
  uint32_t h = hash_key(key);
  uint32_t idx = h % INDEX_CAPACITY;

  for (int i = 0; i < INDEX_CAPACITY; ++i) {
    IndexSlot *slot = &index_table[idx];

    if (slot->used == 0) return -1;

    if (slot->used == 1 && strncmp(slot->key, key, KEY_SIZE) == 0) return slot->offset;

    idx = (idx + 1) % INDEX_CAPACITY;
  }

  return -1;
}

/*
 * Removes key from the index by marking its slot as a tombstone.
 */
void index_delete(const char *key) {
  uint32_t h = hash_key(key);
  uint32_t idx = h % INDEX_CAPACITY;

  for (int i = 0; i < INDEX_CAPACITY; ++i) {
    IndexSlot *slot = &index_table[idx];

    if (slot->used == 0) return;

    if (slot->used == 1 && strncmp(slot->key, key, KEY_SIZE) == 0) {
      slot->used = -1; // tombstone
      return;
    }

    idx = (idx + 1) % INDEX_CAPACITY;
  }
}

/*
 * Inserts or updates a key
 * Reuses tombstones to preserve probe chains
 */
void index_update(const char *key, off_t offset) {
  uint32_t h = hash_key(key);
  uint32_t idx = h % INDEX_CAPACITY;
  int first_tombstone = -1;

  for (int i = 0; i < INDEX_CAPACITY; ++i) {
    IndexSlot *slot = index_table + idx;

    if (slot->used == 1 && strncmp(slot->key, key, KEY_SIZE) == 0) {
      slot->offset = offset;
      return;
    }

    if (slot->used == -1 && first_tombstone == -1) { first_tombstone = idx; }

    if (slot->used == 0) {
      if (first_tombstone != -1) slot = &index_table[first_tombstone];

      strncpy(slot->key, key, KEY_SIZE - 1);
      slot->offset = offset;
      slot->used = 1;
      return;
    }
    idx = (idx + 1) % INDEX_CAPACITY;
  }
}
