"""Small plotting helpers with no notebook-specific state."""

import matplotlib.pyplot as plt
import seaborn as sns


def style():
    sns.set_theme(style="whitegrid", context="notebook")
    plt.rcParams["figure.dpi"] = 120