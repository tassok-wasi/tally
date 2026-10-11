if [ -f tally ]; then
  if [ -f ~/.local/bin/tally ]; then
    rm -f ~/.local/bin/tally
  fi
  cp tally ~/.local/bin
  chmod +x ~/.local/bin/tally
  echo "tally installed"
fi
