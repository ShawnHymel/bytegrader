#include <stdio.h>
#include <stdlib.h>

int main(int argc, char *argv[]) {
    if (argc != 3) {
        fprintf(stderr, "Usage: %s <float1> <float2>\n", argv[0]);
        return 1;
    }
    
    float a = atof(argv[1]);
    float b = atof(argv[2]);
    float sum = a + b;
    
    printf("%.1f\n", sum);
    
    return 0;
}
