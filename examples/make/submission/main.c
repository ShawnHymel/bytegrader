#include <stdio.h>
#include <stdlib.h>

int main(int argc, char *argv[])
{
    // Check if the number of arguments is correct
    if (argc != 3) {
        printf("Usage: %s <num1> <num2>\n", argv[0]);
        return 1;
    }

    // Convert command line arguments to integers and calculate the sum
    // Note: The code assumes that the input is valid integers
    int num1 = atoi(argv[1]);
    int num2 = atoi(argv[2]);
    int sum = num1 + num2;
    printf("%d\n", sum);

    return 0;
}
