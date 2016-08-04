set -e # stop if any of these commands fail

echo "Starting the packager..."
./siphon-packager/packager-daemon.sh start

echo "Starting bundler..."
cd siphon-bundler
./bundler.sh
