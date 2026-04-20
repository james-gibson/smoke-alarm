lezz run ocd-smoke-alarm serve --config configs/samples/dev/1.yaml & echo pid: $!;

lezz run ocd-smoke-alarm serve --config configs/samples/dev/2.yaml & echo pid: $!;

if [ $? -ne 0 ]; then
    echo "process failed"
fi
wait
