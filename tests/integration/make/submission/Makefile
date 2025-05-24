CC = gcc
CFLAGS = -Wall -Wextra -pedantic -std=c99
LDFLAGS = -lm
SOURCES = main.c
OBJECTS = $(SOURCES:.c=.o)
EXECUTABLE = main

# Compile the executable
all: $(EXECUTABLE)
$(EXECUTABLE): $(OBJECTS)
	$(CC) $(LDFLAGS) -o $@ $^

# Compile the object files
%.o: %.c
	$(CC) $(CFLAGS) -c $< -o $@

# Clean up the build
clean:
	rm -f $(OBJECTS) $(EXECUTABLE)

