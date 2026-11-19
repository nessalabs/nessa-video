// C interface to reel's Phonon wrapper; see src/lib.rs.
#ifndef REEL_PHONON_H
#define REEL_PHONON_H
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
typedef struct ReelPhonon ReelPhonon;
typedef bool (*ReelPhononAudio)(const float *pcm, size_t n, void *user);
ReelPhonon *reel_phonon_new(const char *dir, const char *lang);
const char *reel_phonon_voices(const ReelPhonon *h);
bool reel_phonon_speak(ReelPhonon *h, const char *text, const char *voice, float temperature,
                       uint64_t seed, ReelPhononAudio cb, void *user);
const char *reel_phonon_error(const ReelPhonon *h);
void reel_phonon_free(ReelPhonon *h);
#endif
