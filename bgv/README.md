
# BGV

The BGV package provides a unified RNS-accelerated variant of the Fan-Vercauteren version of the Brakerski's scale invariant homomorphic encryption scheme (BFV) and Brakerski-Gentry-Vaikuntanathan (BGV) homomorphic encryption scheme. It enables SIMD modular arithmetic over encrypted vectors or integers.

## Implementation Notes

The proposed implementation is not standard and provides all the functionalities of the BFV and BGV schemes under a unfied scheme.
This enabled by the equivalency between the LSB and MSB encoding when T is coprime to Q (Appendix A of <https://eprint.iacr.org/2013/372>).

### Intuition

The textbook BGV scheme encodes the plaintext in the LSB and the encryption is done by scaling the error by $T$:

$$\textsf{Encrypt}_{s}(\textsf{Encode}(m)) = [-as + m + Te, a]_{Q_{\ell}}$$ 

where $Q_{\ell} = \prod_{i=0}^{L} q_{i}$ in the RNS variant of the scheme.


 The decoding process is then carried out by taking the decrypted plaintext $[m + Te]_{Q_{\ell}}$ modulo $T$ which vanishes the error.

We observe that the only non-linear part of the BGV scheme is its modulus switching operation and that this operation is identical to a CKKS-style rescaling (quantization of the ciphertext by $\frac{1}{q_{\ell}}$) with a pre- and post-processing:

1) Multiply the ciphertext by $T^{-1}\mod Q_{\ell}$ (switch from LSB to MSB encoding)

$$T^{-1} \cdot [-as + m + eT, a]_{Q_{\ell}}\rightarrow[-bs + mT^{-1} + e, b]_{Q_{\ell}}$$

2) Apply the Full-RNS CKKS-style rescaling (division by $q_{\ell} = Q_{\ell}/Q_{\ell-1}$):

$$q_{\ell}^{-1}\cdot[-bs + mT^{-1} + e, b]_{Q_{\ell}}\rceil\rightarrow[-cs + mq_{\ell}^{-1}T^{-1} + \lfloor e/q_{\ell} + e_{\textsf{round}}, c]_{Q_{\ell-1}}$$

3) Multiply the ciphertext by $T \mod Q_{\ell-1}$ (switch from MSB to LSB encoding)

$$T\cdot[-cs + mq_{\ell}^{-1}T^{-1} + \lfloor e/q_{\ell}\rceil + e_{\textsf{round}}, c]_{Q_{\ell-1}}\rightarrow[-ds + mq_{\ell}^{-1} + T(\lfloor e/q_{\ell}\rceil + e_{\textsf{round}}), d]_{Q_{\ell-1}}$$

The process returns a new ciphertext modulo $Q_{\ell-1}$ where the error has been quantized by $q_{\ell}$ and the message multiplied by a factor of $q_{\ell}^{-1} \mod T$.

Since the modulus switch is the only non-linear part of the BGV scheme, we can move steps 1) and 2) to the encoding and decoding steps respectively, i.e. instead of scaling the error during the encryption by $T$ we scale the plaintext by $T^{-1}\mod Q_{\ell}$ during the encoding.

The tensoring operations have to be slightly modified to take into account the additional multiples of $T^{-1}$ (but this can be done for free when operands are switched in the Montgomery domain).

### Functionalities

The above change enables an implementation of the BGV scheme with an MSB encoding, which is essentially the BFV scheme. In other words, if $T$ is coprime with $Q$ then the BFV and BGV encoding (and thus scheme) are indistinguishable up to a plaintext scaling factor of $T^{-1}\mod Q$. 

This unified scheme can also be seen as a variant of the BGV scheme with two tensoring operations:
- The BGV-style tensoring with a noise growth proportional to the current noise
- The BFV-style tensoring with a noise growth invariant to the current noise