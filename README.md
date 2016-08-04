siphon-bundler
==============

Building
--------

Install Go 1.5.1 (you'll need it to run the tests):

https://storage.googleapis.com/golang/go1.5.1.darwin-amd64.pkg

Make sure you have Docker Toolbox installed:

https://www.docker.com/toolbox

How to run the bundler on your local machine
--------------------------------------------

First, run the `Docker Quickstart Terminal` script that came with
Docker Toolbox, as explained here:

http://docs.docker.com/mac/step_one/

You can close the terminal when it's finished setting up. Now run the
following script to spin up containers in your local VirtualBox VM:

    $ ./run-local-containers.sh

Pressing *ctrl-c* will stop the containers.

To run `psql` and inspect the postgres DB, make sure the containers are
running and type;

    $ ./run-local-containers.sh --psql

Running tests
-------------

The tests are written in Python 3, so you will need a virtual environment:

    $ mkvirtualenv --python=`which python3` siphon-bundler
    $ workon siphon-bundler

Install the dependencies:

    $ cd /path/to/this/cloned/repo
    $ pip install -r test_requirements.txt

You will also need PostgreSQL locally:

    $ brew install postgresql

Run the tests:

    $ ./run-tests.sh

Deploying
---------

You should work on new features in a separate branch:

    $ git checkout -b my-new-feature

When you think it won't break, merge into the `staging` branch and the
orchestration server will deploy it for you:

    $ git checkout staging
    $ git merge my-new-feature
    $ git push origin staging
