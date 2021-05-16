Add the following to your `.bashrc` profile to generate a lisp file
that defines all singular aliases. (Make sure this line is after the
emacs command is loaded).

```bash
e el > ~/emacs_aliases.el
```

Then, in `~/.emacs`, add the following line to load all of the aliases:

```
(load "~/emacs_aliases.el)
```

The shortcut for going to an alias file is `C-x C-j`.
