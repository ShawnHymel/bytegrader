# Creating a Grader

This guide walks you through the process of creating your own, custom grader for ByteGrader.

## Recommended: Private Repo

You should keep your graders and solutions private to avoid students finding them and using them to cheat. Yes, they can just use AI for a lot of code, but we can at least keep the answer key out of reach.

Create a private GitHub repository. Suggested name: `course-<NAME_OF_COURSE>-assignments`

Recommended folder structure:

```
TODO
```

## Create Dockerfile

Every grader runs in its own Docker container. As a result, you'll need to create a Docker image (via a Dockerfile). Use one of the example Dockerfiles in *graders/* as a starting point. Note that the grader requires the [python-magic](https://pypi.org/project/python-magic/) package, and it needs the following directories in root (*/*):

* **/assignment** - Holds your custom grading scripts (*grader.py* and any supporting files)
* **/grader** - Holds the grader framework files and entrypoint (*main.py* and *result.py*)
* **/results** - Where the results (*output.json*) are stored
* **/submission** - ByteGrader will copy the student's submission here with the name *submission.zip*
* **/workspace** - Student submitted files are unzipped into this directory.

A note about */workspace*:

* For local testing, files are unzipped directly to */workspace*
* In production, files are unzipped to */workspace/jobs/{job_id}*

In production, files are passed from the main server (API layer) container and the grader container through a shared volume. The */workspace/jobs/{job_id}* is needed (for now) due to how volumes work in Docker. This





